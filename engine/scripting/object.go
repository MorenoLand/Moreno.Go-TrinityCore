package scripting

import (
	"context"
	"fmt"
	"math"
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

func objectIndex(state *lua.State) int {
	object := lua.CheckUserData(state, 1, objectMetaTable).(*Object)
	name := lua.CheckString(state, 2)
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
			other, err := objectArgument(args)
			if err != nil {
				return nil, err
			}
			distance := objectDistance(object, other, strings.HasSuffix(name, "2d"))
			return []any{distance}, nil
		}, true
	case "IsWithinDist", "IsWithinDistInMap", "IsWithinDist3d", "IsInRange", "IsInRange3d":
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
			return []any{float64(objectDistance(object, other, name == "IsWithinDist3d")) <= distance}, nil
		}, true
	case "IsWithinDist2d", "IsInRange2d":
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
			return []any{float64(objectDistance(object, other, true)) <= distance}, nil
		}, true
	case "GetAngle":
		return func(_ context.Context, args []any) ([]any, error) {
			other, err := objectArgument(args)
			if err != nil {
				return nil, err
			}
			x1, y1 := objectFloat(object, "X"), objectFloat(object, "Y")
			x2, y2 := objectFloat(other, "X"), objectFloat(other, "Y")
			return []any{float32(math.Atan2(float64(y2-y1), float64(x2-x1)))}, nil
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
