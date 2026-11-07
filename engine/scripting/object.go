package scripting

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Shopify/go-lua"
)

const objectMetaTable = "MorenoCore.Object"

func PushObject(state *lua.State, object *Object) {
	state.PushUserData(object)
	lua.SetMetaTableNamed(state, objectMetaTable)
}

func objectEqual(state *lua.State) int {
	left := lua.CheckUserData(state, 1, objectMetaTable).(*Object)
	right := lua.CheckUserData(state, 2, objectMetaTable).(*Object)
	state.PushBoolean(left == right)
	return 1
}

// instanceDataStoreKey is the Lua registry slot holding the per-instance
// data store: one real Lua table per (map, instance) key, so Eluna
// instance scripts read and write the same table across hooks, exactly
// like ElunaInstanceAI's Lua data table (LuaEngine/MapMethods.h
// GetInstanceData, ElunaInstanceAI.cpp GetData/SetData). The table lives
// in the registry for the Runtime's lifetime; the Go InstanceData object
// is only its userdata handle, keyed by its MapId/InstanceId fields.
const instanceDataStoreKey = "MorenoCore.InstanceDataStore"

func instanceDataKey(object *Object) string {
	mapID, _ := objectUint32(object, "MapId")
	instanceID, _ := objectUint32(object, "InstanceId")
	return strconv.Itoa(int(mapID)) + ":" + strconv.Itoa(int(instanceID))
}

// pushInstanceDataTable leaves the (map, instance) data table on top of
// the stack and reports true, or leaves nil and reports false when no
// table exists and create is false. With create it materializes the
// store and the instance table first. Exactly one value is left in
// every path: the store table is removed before returning, so direct
// callers outside a Lua call frame (Marshal/UnmarshalInstanceData)
// can balance with a single Pop.
func pushInstanceDataTable(state *lua.State, object *Object, create bool) bool {
	state.Field(lua.RegistryIndex, instanceDataStoreKey)
	if !state.IsTable(-1) {
		state.Pop(1)
		if !create {
			state.PushNil()
			return false
		}
		state.NewTable()
		state.SetField(lua.RegistryIndex, instanceDataStoreKey)
		state.Field(lua.RegistryIndex, instanceDataStoreKey)
	}
	state.PushString(instanceDataKey(object))
	state.RawGet(-2)
	if !state.IsTable(-1) {
		state.Pop(1)
		if !create {
			state.PushNil()
			state.Remove(-2)
			return false
		}
		state.NewTable()
		state.PushString(instanceDataKey(object))
		state.PushValue(-2)
		state.RawSet(-4)
	}
	state.Remove(-2)
	return true
}

// pushInstanceDataField leaves the named field of the instance data
// table on top of the stack, or nil when the table or field is absent.
func pushInstanceDataField(state *lua.State, object *Object, name string) {
	if !pushInstanceDataTable(state, object, false) {
		return
	}
	state.PushString(name)
	state.RawGet(-2)
}

func objectNewIndex(state *lua.State) int {
	object := lua.CheckUserData(state, 1, objectMetaTable).(*Object)
	name := lua.CheckString(state, 2)
	if object.Type != "InstanceData" {
		lua.Errorf(state, "attempt to index a userdata value")
	}
	if !pushInstanceDataTable(state, object, true) {
		lua.Errorf(state, "instance data table unavailable")
	}
	state.PushValue(2)
	state.PushValue(3)
	state.RawSet(-3)
	_ = name
	return 0
}

func objectIndex(state *lua.State) int {
	object := lua.CheckUserData(state, 1, objectMetaTable).(*Object)
	name := lua.CheckString(state, 2)
	if object.Type == "InstanceData" {
		pushInstanceDataField(state, object, name)
		return 1
	}
	method, ok := object.Methods[name]
	if !ok {
		method, ok = genericObjectMethod(object, name)
	}
	if ok {
		state.PushGoFunction(func(call *lua.State) int {
			args := make([]any, 0, call.Top()-1)
			for index := 2; index <= call.Top(); index++ {
				args = append(args, luaValue(call, index))
			}
			values, err := method(context.Background(), args)
			if err != nil {
				lua.Errorf(call, "%s: %s", name, err)
			}
			for _, value := range values {
				pushValue(call, value)
			}
			return len(values)
		})
		return 1
	}
	if value, ok := object.Fields[name]; ok {
		pushValue(state, value)
		return 1
	}
	state.PushNil()
	return 1
}

func genericObjectMethod(object *Object, name string) (ObjectMethod, bool) {
	switch name {
	case "GetName":
		return func(context.Context, []any) ([]any, error) { return []any{object.Fields["Name"]}, nil }, true
	case "GetEntry":
		return func(context.Context, []any) ([]any, error) {
			value, _ := objectUint32(object, "Entry")
			return []any{value}, nil
		}, true
	case "GetGUID":
		return func(context.Context, []any) ([]any, error) {
			value, _ := objectUint64(object, "GUID")
			return []any{value}, nil
		}, true
	case "GetGUIDLow":
		return func(context.Context, []any) ([]any, error) {
			value, _ := objectUint64(object, "GUID")
			return []any{guidLow(value)}, nil
		}, true
	case "GetTypeId":
		return func(context.Context, []any) ([]any, error) { return []any{objectTypeID(object.Type)}, nil }, true
	case "IsInWorld":
		return func(context.Context, []any) ([]any, error) {
			value, ok := object.Fields["InWorld"].(bool)
			if !ok {
				value = true
			}
			return []any{value}, nil
		}, true
	case "GetMapId":
		return func(context.Context, []any) ([]any, error) { return []any{objectMapID(object)}, nil }, true
	case "GetInstanceId":
		return func(context.Context, []any) ([]any, error) {
			value, _ := objectUint32(object, "InstanceId")
			return []any{value}, nil
		}, true
	case "GetX", "GetY", "GetZ", "GetO":
		coordinate := map[string]string{"GetX": "X", "GetY": "Y", "GetZ": "Z", "GetO": "Orientation"}[name]
		return func(context.Context, []any) ([]any, error) { return []any{objectFloat(object, coordinate)}, nil }, true
	case "GetLocation":
		return func(context.Context, []any) ([]any, error) {
			return []any{objectFloat(object, "X"), objectFloat(object, "Y"), objectFloat(object, "Z"), objectFloat(object, "Orientation")}, nil
		}, true
	case "GetHealth", "GetMaxHealth", "GetLevel", "GetRace", "GetClass", "GetGender", "GetPower", "GetMaxPower", "GetPowerType":
		field := map[string]string{"GetHealth": "Health", "GetMaxHealth": "MaxHealth", "GetLevel": "Level", "GetRace": "Race", "GetClass": "Class", "GetGender": "Gender", "GetPower": "Power", "GetMaxPower": "MaxPower", "GetPowerType": "PowerType"}[name]
		return func(context.Context, []any) ([]any, error) {
			value, _ := objectUint32(object, field)
			return []any{value}, nil
		}, true
	case "GetHealthPct":
		return func(context.Context, []any) ([]any, error) {
			health, _ := objectUint32(object, "Health")
			maxHealth, _ := objectUint32(object, "MaxHealth")
			if maxHealth == 0 {
				return []any{float32(0)}, nil
			}
			return []any{float32(health) * 100 / float32(maxHealth)}, nil
		}, true
	case "HealthBelowPct", "HealthAbovePct", "CountPctFromCurHealth", "CountPctFromMaxHealth":
		// Unit::HealthBelowPct/HealthAbovePct/CountPctFromCurHealth/
		// CountPctFromMaxHealth (UnitMethods.h, Unit.h:872-878):
		// Below is health < CountPctFromMaxHealth(pct), Above is
		// health > CountPctFromMaxHealth(pct), and CountPct is
		// CalculatePct (Util.h:66-70), base * float(pct) / 100
		// truncated to uint32. The unit builders define these over
		// live/snapshot health; this fallback serves objects without
		// their own builder.
		return func(_ context.Context, args []any) ([]any, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("%s requires a percent", name)
			}
			pct, ok := numericValue(args[0])
			if !ok {
				return nil, fmt.Errorf("%s requires a numeric percent", name)
			}
			health, _ := objectUint32(object, "Health")
			maxHealth, _ := objectUint32(object, "MaxHealth")
			countPct := func(base uint32) uint32 {
				return uint32(float32(base) * float32(int32(pct)) / 100)
			}
			switch name {
			case "HealthBelowPct":
				return []any{health < countPct(maxHealth)}, nil
			case "HealthAbovePct":
				return []any{health > countPct(maxHealth)}, nil
			case "CountPctFromCurHealth":
				return []any{countPct(health)}, nil
			default:
				return []any{countPct(maxHealth)}, nil
			}
		}, true
	case "GetPowerPct":
		return func(context.Context, []any) ([]any, error) {
			power, _ := objectUint32(object, "Power")
			maxPower, _ := objectUint32(object, "MaxPower")
			if maxPower == 0 {
				return []any{float32(0)}, nil
			}
			return []any{float32(power) * 100 / float32(maxPower)}, nil
		}, true
	case "IsAlive", "IsDead", "IsFullHealth":
		return func(context.Context, []any) ([]any, error) {
			health, _ := objectUint32(object, "Health")
			maxHealth, _ := objectUint32(object, "MaxHealth")
			switch name {
			case "IsDead":
				return []any{health == 0}, nil
			case "IsFullHealth":
				return []any{maxHealth > 0 && health >= maxHealth}, nil
			default:
				return []any{health > 0}, nil
			}
		}, true
	case "IsInCombat":
		return func(context.Context, []any) ([]any, error) {
			value, _ := object.Fields["InCombat"].(bool)
			return []any{value}, nil
		}, true
	case "ToPlayer", "ToCreature", "ToGameObject", "ToUnit", "ToCorpse":
		return func(context.Context, []any) ([]any, error) {
			if objectMatchesType(object.Type, name[2:]) {
				return []any{object}, nil
			}
			return []any{nil}, nil
		}, true
	case "IsInMap":
		return func(_ context.Context, args []any) ([]any, error) {
			other, err := objectArgument(args)
			if err != nil {
				return nil, err
			}
			return []any{objectMapID(object) == objectMapID(other)}, nil
		}, true
	case "GetDistance", "GetExactDistance", "GetDistance2d", "GetExactDistance2d":
		return func(_ context.Context, args []any) ([]any, error) {
			twoDimensional := strings.HasSuffix(name, "2d")
			if len(args) == 0 {
				return nil, fmt.Errorf("%s requires an object or coordinates", name)
			}
			if other, ok := args[0].(*Object); ok && other != nil {
				distance := objectDistance(object, other, twoDimensional)
				return []any{distance}, nil
			}
			// Point form (WorldObjectMethods.h: GetDistance(x, y, z),
			// GetDistance2d(x, y) and the Exact twins): previously the
			// object-only path errored on a numeric first argument.
			// Generic objects carry no combat reach in Fields, so the
			// point form uses the same center-distance model as Go's
			// existing object form (the C++ reach subtraction is
			// unmodeled here, matching the object form already in tree).
			need := 3
			if twoDimensional {
				need = 2
			}
			if len(args) < need {
				return nil, fmt.Errorf("%s requires an object or %d coordinates", name, need)
			}
			point := make([]float64, need)
			for i := 0; i < need; i++ {
				value, ok := numericValue(args[i])
				if !ok {
					return nil, fmt.Errorf("coordinate must be numeric")
				}
				point[i] = value
			}
			dx := float64(objectFloat(object, "X")) - point[0]
			dy := float64(objectFloat(object, "Y")) - point[1]
			if twoDimensional {
				return []any{float32(math.Hypot(dx, dy))}, nil
			}
			dz := float64(objectFloat(object, "Z")) - point[2]
			return []any{float32(math.Sqrt(dx*dx + dy*dy + dz*dz))}, nil
		}, true
	case "IsWithinDist", "IsWithinDistInMap":
		return func(_ context.Context, args []any) ([]any, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("%s requires an object and distance", name)
			}
			other, err := objectArgument(args[:1])
			if err != nil {
				return nil, err
			}
			distance, ok := numericValue(args[1])
			if !ok {
				return nil, fmt.Errorf("distance must be numeric")
			}
			if name == "IsWithinDistInMap" && objectMapID(object) != objectMapID(other) {
				return []any{false}, nil
			}
			return []any{float64(objectDistance(object, other, false)) <= distance}, nil
		}, true
	case "IsWithinDist3d", "IsWithinDist2d":
		// Point forms (WorldObjectMethods.h): IsWithinDist3d(x, y, z, dist)
		// is IsInDist(x, y, z, dist + own reach) and IsWithinDist2d(x, y,
		// dist) the 2D twin. Generic objects carry no reach in Fields,
		// so the reach term is 0 and the check is exact distance <= dist.
		// Previously these fell into the object+distance shape and errored
		// on a numeric first argument.
		return func(_ context.Context, args []any) ([]any, error) {
			twoDimensional := name == "IsWithinDist2d"
			need := 4
			if twoDimensional {
				need = 3
			}
			if len(args) < need {
				return nil, fmt.Errorf("%s requires coordinates and a distance", name)
			}
			point := make([]float64, need)
			for i := 0; i < need; i++ {
				value, ok := numericValue(args[i])
				if !ok {
					return nil, fmt.Errorf("coordinate or distance must be numeric")
				}
				point[i] = value
			}
			dx := float64(objectFloat(object, "X")) - point[0]
			dy := float64(objectFloat(object, "Y")) - point[1]
			var dist float64
			if twoDimensional {
				dist = math.Hypot(dx, dy)
			} else {
				dz := float64(objectFloat(object, "Z")) - point[2]
				dist = math.Sqrt(dx*dx + dy*dy + dz*dz)
			}
			return []any{dist <= point[need-1]}, nil
		}, true
	case "IsInRange":
		// Object band form (Object.cpp:1329): IsInRange(target, minRange,
		// maxRange, is3D = true) checks the center distance against the
		// band — below min (when min > 0) fails, at or above max fails
		// (strict upper bound). Previously this fell into the
		// object+single-distance shape, which is IsWithinDist semantics,
		// not the C++ band. Reaches are uncarried on generic objects, so
		// the sizefactor term is 0.
		return func(_ context.Context, args []any) ([]any, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("IsInRange requires an object, minRange and maxRange")
			}
			other, err := objectArgument(args[:1])
			if err != nil {
				return nil, err
			}
			minRange, ok := numericValue(args[1])
			if !ok {
				return nil, fmt.Errorf("minRange must be numeric")
			}
			maxRange, ok := numericValue(args[2])
			if !ok {
				return nil, fmt.Errorf("maxRange must be numeric")
			}
			twoDimensional := false
			if len(args) > 3 {
				if is3D, ok := args[3].(bool); ok {
					twoDimensional = !is3D
				}
			}
			dist := float64(objectDistance(object, other, twoDimensional))
			if minRange > 0 && dist < minRange {
				return []any{false}, nil
			}
			return []any{dist < maxRange}, nil
		}, true
	case "IsInRange2d", "IsInRange3d":
		// Point band forms (Object.cpp:1354/1374): IsInRange2d(x, y, min,
		// max) and IsInRange3d(x, y, z, min, max) with the same band
		// rule as IsInRange and a zero sizefactor (see above). Previously
		// these fell into the object+single-distance shape and errored
		// on a numeric first argument.
		return func(_ context.Context, args []any) ([]any, error) {
			twoDimensional := name == "IsInRange2d"
			need := 5
			if twoDimensional {
				need = 4
			}
			if len(args) < need {
				return nil, fmt.Errorf("%s requires coordinates, minRange and maxRange", name)
			}
			point := make([]float64, need)
			for i := 0; i < need; i++ {
				value, ok := numericValue(args[i])
				if !ok {
					return nil, fmt.Errorf("coordinate or range must be numeric")
				}
				point[i] = value
			}
			dx := float64(objectFloat(object, "X")) - point[0]
			dy := float64(objectFloat(object, "Y")) - point[1]
			var dist float64
			if twoDimensional {
				dist = math.Hypot(dx, dy)
			} else {
				dz := float64(objectFloat(object, "Z")) - point[2]
				dist = math.Sqrt(dx*dx + dy*dy + dz*dz)
			}
			minRange, maxRange := point[need-2], point[need-1]
			if minRange > 0 && dist < minRange {
				return []any{false}, nil
			}
			return []any{dist < maxRange}, nil
		}, true
	case "GetAngle":
		return func(_ context.Context, args []any) ([]any, error) {
			x1, y1 := objectFloat(object, "X"), objectFloat(object, "Y")
			if len(args) == 0 {
				return nil, fmt.Errorf("GetAngle requires an object or coordinates")
			}
			if other, ok := args[0].(*Object); ok && other != nil {
				x2, y2 := objectFloat(other, "X"), objectFloat(other, "Y")
				return []any{float32(math.Atan2(float64(y2-y1), float64(x2-x1)))}, nil
			}
			// Point form (WorldObjectMethods.h: GetAngle(x, y)): previously
			// the object-only path errored on a numeric first argument.
			if len(args) < 2 {
				return nil, fmt.Errorf("GetAngle requires an object or x and y coordinates")
			}
			x2, ok := numericValue(args[0])
			if !ok {
				return nil, fmt.Errorf("x coordinate must be numeric")
			}
			y2, ok := numericValue(args[1])
			if !ok {
				return nil, fmt.Errorf("y coordinate must be numeric")
			}
			return []any{float32(math.Atan2(y2-float64(y1), x2-float64(x1)))}, nil
		}, true
	}
	return nil, false
}

func objectTypeID(objectType string) uint8 {
	switch objectType {
	case "Item":
		return 1
	case "Container":
		return 2
	case "Creature", "Unit":
		return 3
	case "Player":
		return 4
	case "GameObject":
		return 5
	case "DynamicObject":
		return 6
	case "Corpse":
		return 7
	default:
		return 0
	}
}

func objectMatchesType(objectType, target string) bool {
	switch target {
	case "Player":
		return objectType == "Player"
	case "Creature":
		return objectType == "Creature"
	case "GameObject":
		return objectType == "GameObject"
	case "Corpse":
		return objectType == "Corpse"
	case "Unit":
		return objectType == "Player" || objectType == "Creature"
	default:
		return false
	}
}

func objectMapID(object *Object) uint32 {
	if value, ok := objectUint32(object, "MapId"); ok {
		return value
	}
	value, _ := objectUint32(object, "Map")
	return value
}

func objectFloat(object *Object, field string) float32 {
	if object == nil {
		return 0
	}
	value, ok := object.Fields[field]
	if !ok {
		return 0
	}
	if number, ok := numericValue(value); ok {
		return float32(number)
	}
	return 0
}

func objectArgument(args []any) (*Object, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("object argument is required")
	}
	object, ok := args[0].(*Object)
	if !ok || object == nil {
		return nil, fmt.Errorf("object argument expected")
	}
	return object, nil
}

func objectDistance(first, second *Object, twoDimensional bool) float32 {
	dx := float64(objectFloat(first, "X") - objectFloat(second, "X"))
	dy := float64(objectFloat(first, "Y") - objectFloat(second, "Y"))
	if twoDimensional {
		return float32(math.Hypot(dx, dy))
	}
	dz := float64(objectFloat(first, "Z") - objectFloat(second, "Z"))
	return float32(math.Sqrt(dx*dx + dy*dy + dz*dz))
}

func numericValue(value any) (float64, bool) {
	switch value := value.(type) {
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	case UInt64:
		return float64(uint64(value)), true
	case Int64:
		return float64(int64(value)), true
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case float32:
		return float64(value), true
	case float64:
		return value, true
	default:
		return 0, false
	}
}

func pushValue(state *lua.State, value any) error {
	switch value := value.(type) {
	case nil:
		state.PushNil()
	case bool:
		state.PushBoolean(value)
	case string:
		state.PushString(value)
	case []byte:
		state.PushString(string(value))
	case int:
		state.PushInteger(value)
	case int8:
		state.PushInteger(int(value))
	case int16:
		state.PushInteger(int(value))
	case int32:
		state.PushInteger(int(value))
	case int64:
		state.PushNumber(float64(value))
	case uint:
		state.PushUnsigned(value)
	case uint8:
		state.PushUnsigned(uint(value))
	case uint16:
		state.PushUnsigned(uint(value))
	case uint32:
		state.PushUnsigned(uint(value))
	case uint64:
		pushUInt64(state, value)
	case float32:
		state.PushNumber(float64(value))
	case float64:
		state.PushNumber(value)
	case *Query:
		pushQuery(state, value)
	case *Packet:
		pushPacket(state, value)
	case *Object:
		PushObject(state, value)
	case []any:
		state.NewTable()
		for i, item := range value {
			if err := pushValue(state, item); err != nil {
				return err
			}
			state.RawSetInt(-2, i+1)
		}
	default:
		return fmt.Errorf("unsupported Lua value %T", value)
	}
	return nil
}

func luaValue(state *lua.State, index int) any {
	if state.IsNil(index) {
		return nil
	}
	if state.IsBoolean(index) {
		return state.ToBoolean(index)
	}
	if state.IsNumber(index) {
		value, _ := state.ToNumber(index)
		return value
	}
	if state.IsString(index) {
		value, _ := state.ToString(index)
		return value
	}
	if value := state.ToUserData(index); value != nil {
		return value
	}
	return state.ToValue(index)
}
