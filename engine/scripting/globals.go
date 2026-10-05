package scripting

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/version"
	"github.com/Shopify/go-lua"
)

const (
	guidHighItem        uint16 = 0x4000
	guidHighPlayer      uint16 = 0x0000
	guidHighGameObject  uint16 = 0xF110
	guidHighTransport   uint16 = 0xF120
	guidHighUnit        uint16 = 0xF130
	guidHighPet         uint16 = 0xF140
	guidHighVehicle     uint16 = 0xF150
	guidHighDynamic     uint16 = 0xF100
	guidHighCorpse      uint16 = 0xF101
	guidHighMOTransport uint16 = 0x1FC0
	guidHighInstance    uint16 = 0x1F40
	guidHighGroup       uint16 = 0x1F50
)

const uint64MetaTable = "MorenoCore.UInt64"
const int64MetaTable = "MorenoCore.Int64"

type UInt64 uint64
type Int64 int64

func (r *Runtime) getLuaEngine(state *lua.State) int {
	state.PushString("ElunaEngine")
	return 1
}

func (r *Runtime) getCoreName(state *lua.State) int {
	name := r.config.CoreName
	if name == "" {
		name = version.Product
	}
	state.PushString(name)
	return 1
}

func (r *Runtime) getRealmID(state *lua.State) int {
	realmID := r.config.RealmID
	if realmID == 0 {
		realmID = 1
	}
	state.PushUnsigned(uint(realmID))
	return 1
}

func (r *Runtime) getCoreVersion(state *lua.State) int {
	value := r.config.CoreVersion
	if value == "" {
		value = version.String()
	}
	state.PushString(value)
	return 1
}

func (r *Runtime) getGameTime(state *lua.State) int {
	state.PushUnsigned(uint(time.Now().Unix()))
	return 1
}

func (r *Runtime) getQuest(state *lua.State) int {
	questID := checkLuaUint64(state, 1)
	if r.config.WorldDatabase == nil || questID > math.MaxUint32 {
		state.PushNil()
		return 1
	}
	var id, level, minLevel, flags, nextID, prevID, questType int64
	var title string
	err := r.config.WorldDatabase.QueryRow("SELECT ID, COALESCE(LogTitle, ''), COALESCE(QuestLevel, 0), COALESCE(MinLevel, 0), COALESCE(Flags, 0), COALESCE(RewardNextQuest, 0), COALESCE(PrevQuestId, 0), COALESCE(Type, 0) FROM quest_template WHERE ID = ?", uint32(questID)).Scan(&id, &title, &level, &minLevel, &flags, &nextID, &prevID, &questType)
	if err != nil {
		err = r.config.WorldDatabase.QueryRow("SELECT ID, COALESCE(QuestLevel, 0), COALESCE(MinLevel, 0), COALESCE(Flags, 0) FROM quest_template WHERE ID = ?", uint32(questID)).Scan(&id, &level, &minLevel, &flags)
	}
	if err != nil {
		state.PushNil()
		return 1
	}
	methods := map[string]ObjectMethod{}
	methods["GetId"] = func(context.Context, []any) ([]any, error) { return []any{uint32(id)}, nil }
	methods["GetLevel"] = func(context.Context, []any) ([]any, error) { return []any{uint32(level)}, nil }
	methods["GetMinLevel"] = func(context.Context, []any) ([]any, error) { return []any{uint32(minLevel)}, nil }
	methods["GetFlags"] = func(context.Context, []any) ([]any, error) { return []any{uint32(flags)}, nil }
	methods["GetNextQuestId"] = func(context.Context, []any) ([]any, error) { return []any{int32(nextID)}, nil }
	methods["GetPrevQuestId"] = func(context.Context, []any) ([]any, error) { return []any{int32(prevID)}, nil }
	methods["GetType"] = func(context.Context, []any) ([]any, error) { return []any{uint32(questType)}, nil }
	methods["HasFlag"] = func(_ context.Context, args []any) ([]any, error) {
		flag, err := objectArgumentUint32(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{uint32(flags)&flag != 0}, nil
	}
	methods["IsDaily"] = func(context.Context, []any) ([]any, error) { return []any{uint32(flags)&0x1000 != 0}, nil }
	methods["IsRepeatable"] = func(context.Context, []any) ([]any, error) { return []any{uint32(flags)&0x9000 != 0}, nil }
	PushObject(state, &Object{Type: "Quest", Fields: map[string]any{"ID": uint32(id), "Name": title, "Title": title, "Level": uint32(level), "MinLevel": uint32(minLevel), "Flags": uint32(flags)}, Methods: methods})
	return 1
}

func objectArgumentUint32(args []any, index int) (uint32, error) {
	if index >= len(args) {
		return 0, fmt.Errorf("argument %d is required", index+1)
	}
	number, ok := numericValue(args[index])
	if !ok || number < 0 || number > math.MaxUint32 || number != math.Trunc(number) {
		return 0, fmt.Errorf("argument %d must be uint32", index+1)
	}
	return uint32(number), nil
}

func (r *Runtime) getPlayerByGUID(state *lua.State) int {
	guid := checkLuaUint64(state, 1)
	if r.config.PlayerProvider != nil {
		for _, player := range r.config.PlayerProvider() {
			if value, ok := objectUint64(player, "GUID"); ok && value == guid {
				PushObject(state, player)
				return 1
			}
		}
	}
	state.PushNil()
	return 1
}

func (r *Runtime) getGuildByName(state *lua.State) int {
	return r.pushGuildLookup(state, "SELECT guildid, name, leaderguid FROM guild WHERE name = ? LIMIT 1", lua.CheckString(state, 1))
}

func (r *Runtime) getGuildByLeaderGUID(state *lua.State) int {
	return r.pushGuildLookup(state, "SELECT guildid, name, leaderguid FROM guild WHERE leaderguid = ? LIMIT 1", checkLuaUint64(state, 1))
}

// NewGuildObject builds the Lua Guild object surface: the ID, Name,
// LeaderGUID and MemberCount fields plus the GetId/GetName/GetLeaderGUID/
// GetMemberCount methods. It is the Go model of Eluna's Push(guild)
// (LuaEngine/GuildHooks.cpp via ElunaTemplate), shared by the GetGuildBy*
// globals and the world engine's guild event hooks.
func NewGuildObject(guildID uint32, name string, leaderGUID uint64, memberCount uint32) *Object {
	methods := map[string]ObjectMethod{
		"GetId":          func(context.Context, []any) ([]any, error) { return []any{uint32(guildID)}, nil },
		"GetName":        func(context.Context, []any) ([]any, error) { return []any{name}, nil },
		"GetLeaderGUID":  func(context.Context, []any) ([]any, error) { return []any{uint64(leaderGUID)}, nil },
		"GetMemberCount": func(context.Context, []any) ([]any, error) { return []any{uint32(memberCount)}, nil },
	}
	return &Object{Type: "Guild", Fields: map[string]any{"ID": uint32(guildID), "Name": name, "LeaderGUID": uint64(leaderGUID), "MemberCount": uint32(memberCount)}, Methods: methods}
}

func (r *Runtime) pushGuildLookup(state *lua.State, statement string, arg any) int {
	if r.config.CharacterDB == nil {
		state.PushNil()
		return 1
	}
	var guildID, leaderGUID int64
	var name string
	if err := r.config.CharacterDB.QueryRow(statement, arg).Scan(&guildID, &name, &leaderGUID); err != nil {
		state.PushNil()
		return 1
	}
	memberCount := int64(0)
	_ = r.config.CharacterDB.QueryRow("SELECT COUNT(1) FROM guild_member WHERE guildid = ?", guildID).Scan(&memberCount)
	PushObject(state, NewGuildObject(uint32(guildID), name, uint64(leaderGUID), uint32(memberCount)))
	return 1
}

func (r *Runtime) getPlayerCount(state *lua.State) int {
	count := 0
	if r.config.PlayerProvider != nil {
		for _, player := range r.config.PlayerProvider() {
			if player != nil {
				count++
			}
		}
	}
	state.PushUnsigned(uint(count))
	return 1
}

func (r *Runtime) getPlayerGUID(state *lua.State) int {
	pushUInt64(state, makeGlobalGUID(guidHighPlayer, uint32(checkLuaUint64(state, 1)), 0))
	return 1
}

func (r *Runtime) getItemGUID(state *lua.State) int {
	pushUInt64(state, makeGlobalGUID(guidHighItem, uint32(checkLuaUint64(state, 1)), 0))
	return 1
}

func (r *Runtime) getObjectGUID(state *lua.State) int {
	pushUInt64(state, makeMapGUID(guidHighGameObject, uint32(checkLuaUint64(state, 1)), uint32(checkLuaUint64(state, 2))))
	return 1
}

func (r *Runtime) getUnitGUID(state *lua.State) int {
	pushUInt64(state, makeMapGUID(guidHighUnit, uint32(checkLuaUint64(state, 1)), uint32(checkLuaUint64(state, 2))))
	return 1
}

func (r *Runtime) getGUIDLow(state *lua.State) int {
	guid := checkLuaUint64(state, 1)
	state.PushUnsigned(uint(guidLow(guid)))
	return 1
}

func (r *Runtime) getGUIDType(state *lua.State) int {
	guid := checkLuaUint64(state, 1)
	state.PushUnsigned(uint(guid >> 48))
	return 1
}

func (r *Runtime) getGUIDEntry(state *lua.State) int {
	guid := checkLuaUint64(state, 1)
	if guidHasEntry(uint16(guid >> 48)) {
		state.PushUnsigned(uint((guid >> 24) & 0x00FFFFFF))
		return 1
	}
	state.PushUnsigned(0)
	return 1
}

func (r *Runtime) getCurrTime(state *lua.State) int {
	state.PushUnsigned(uint(uint32(time.Now().UnixMilli())))
	return 1
}

func (r *Runtime) getTimeDiff(state *lua.State) int {
	old := uint32(lua.CheckUnsigned(state, 1))
	now := uint32(time.Now().UnixMilli())
	state.PushUnsigned(uint(now - old))
	return 1
}

func (r *Runtime) createInt64(state *lua.State) int {
	value := int64(0)
	if state.Top() > 0 && !state.IsNil(1) {
		if state.IsString(1) {
			parsed, err := strconv.ParseInt(lua.CheckString(state, 1), 10, 64)
			if err != nil {
				lua.ArgumentError(state, 1, "int64 value expected")
			}
			value = parsed
		} else {
			value = checkLuaInt64(state, 1)
		}
	}
	pushInt64(state, value)
	return 1
}

func (r *Runtime) createUint64(state *lua.State) int {
	value := uint64(0)
	if state.Top() > 0 && !state.IsNil(1) {
		if state.IsString(1) {
			parsed, err := strconv.ParseUint(lua.CheckString(state, 1), 10, 64)
			if err != nil {
				lua.ArgumentError(state, 1, "uint64 value expected")
			}
			value = parsed
		} else {
			value = checkLuaUint64(state, 1)
		}
	}
	pushUInt64(state, value)
	return 1
}

func (r *Runtime) createPacket(state *lua.State) int {
	opcode := checkLuaUint64(state, 1)
	if opcode > math.MaxUint32 {
		lua.ArgumentError(state, 1, "opcode must be uint32")
	}
	pushPacket(state, &Packet{Opcode: uint32(opcode)})
	return 1
}

func (r *Runtime) printInfo(state *lua.State) int {
	r.printLua(state, "info")
	return 0
}

func (r *Runtime) printError(state *lua.State) int {
	r.printLua(state, "error")
	return 0
}

func (r *Runtime) printDebug(state *lua.State) int {
	r.printLua(state, "debug")
	return 0
}

func (r *Runtime) printLua(state *lua.State, level string) {
	if r.config.Logger == nil {
		return
	}
	parts := make([]string, 0, state.Top())
	for index := 1; index <= state.Top(); index++ {
		parts = append(parts, fmt.Sprint(luaValue(state, index)))
	}
	r.config.Logger.Log(context.Background(), slogLevel(level), strings.Join(parts, "\t"))
}

func installUInt64MetaTable(state *lua.State) {
	lua.NewMetaTable(state, uint64MetaTable)
	lua.SetFunctions(state, []lua.RegistryFunction{
		{Name: "__tostring", Function: uint64String},
		{Name: "__eq", Function: uint64Equal},
		{Name: "__lt", Function: uint64Less},
		{Name: "__le", Function: uint64LessEqual},
		{Name: "__add", Function: uint64Add},
		{Name: "__sub", Function: uint64Sub},
	}, 0)
	state.Pop(1)
	installInt64MetaTable(state)
}

func installInt64MetaTable(state *lua.State) {
	lua.NewMetaTable(state, int64MetaTable)
	lua.SetFunctions(state, []lua.RegistryFunction{
		{Name: "__tostring", Function: int64String},
		{Name: "__eq", Function: int64Equal},
		{Name: "__lt", Function: int64Less},
		{Name: "__le", Function: int64LessEqual},
		{Name: "__add", Function: int64Add},
		{Name: "__sub", Function: int64Sub},
	}, 0)
	state.Pop(1)
}

func pushUInt64(state *lua.State, value uint64) {
	state.PushUserData(UInt64(value))
	lua.SetMetaTableNamed(state, uint64MetaTable)
}

func pushInt64(state *lua.State, value int64) {
	state.PushUserData(Int64(value))
	lua.SetMetaTableNamed(state, int64MetaTable)
}

func checkLuaUint64(state *lua.State, index int) uint64 {
	if value, ok := luaUint64Value(state, index); ok {
		return value
	}
	lua.ArgumentError(state, index, "unsigned integer expected")
	return 0
}

func checkLuaInt64(state *lua.State, index int) int64 {
	if value := state.ToUserData(index); value != nil {
		switch value := value.(type) {
		case Int64:
			return int64(value)
		case *Int64:
			return int64(*value)
		}
	}
	if number, ok := state.ToNumber(index); ok && number >= math.MinInt64 && number <= math.MaxInt64 && number == math.Trunc(number) {
		return int64(number)
	}
	lua.ArgumentError(state, index, "signed integer expected")
	return 0
}

func luaUint64Value(state *lua.State, index int) (uint64, bool) {
	if value := state.ToUserData(index); value != nil {
		switch value := value.(type) {
		case UInt64:
			return uint64(value), true
		case *UInt64:
			return uint64(*value), true
		}
	}
	if number, ok := state.ToNumber(index); ok && number >= 0 && number <= math.MaxUint64 && number == math.Trunc(number) {
		return uint64(number), true
	}
	return 0, false
}

func uint64Operand(state *lua.State, index int) uint64 { return checkLuaUint64(state, index) }

func uint64String(state *lua.State) int {
	state.PushString(strconv.FormatUint(uint64Operand(state, 1), 10))
	return 1
}

func uint64Equal(state *lua.State) int {
	state.PushBoolean(uint64Operand(state, 1) == uint64Operand(state, 2))
	return 1
}

func uint64Less(state *lua.State) int {
	state.PushBoolean(uint64Operand(state, 1) < uint64Operand(state, 2))
	return 1
}

func uint64LessEqual(state *lua.State) int {
	state.PushBoolean(uint64Operand(state, 1) <= uint64Operand(state, 2))
	return 1
}

func uint64Add(state *lua.State) int {
	pushUInt64(state, uint64Operand(state, 1)+uint64Operand(state, 2))
	return 1
}

func uint64Sub(state *lua.State) int {
	pushUInt64(state, uint64Operand(state, 1)-uint64Operand(state, 2))
	return 1
}

func int64String(state *lua.State) int {
	state.PushString(strconv.FormatInt(checkLuaInt64(state, 1), 10))
	return 1
}

func int64Value(state *lua.State, index int) int64 { return checkLuaInt64(state, index) }

func int64Equal(state *lua.State) int {
	state.PushBoolean(int64Value(state, 1) == int64Value(state, 2))
	return 1
}

func int64Less(state *lua.State) int {
	state.PushBoolean(int64Value(state, 1) < int64Value(state, 2))
	return 1
}

func int64LessEqual(state *lua.State) int {
	state.PushBoolean(int64Value(state, 1) <= int64Value(state, 2))
	return 1
}

func int64Add(state *lua.State) int {
	pushInt64(state, int64Value(state, 1)+int64Value(state, 2))
	return 1
}

func int64Sub(state *lua.State) int {
	pushInt64(state, int64Value(state, 1)-int64Value(state, 2))
	return 1
}

func luaBitAnd(state *lua.State) int {
	state.PushUnsigned(uint(lua.CheckUnsigned(state, 1) & lua.CheckUnsigned(state, 2)))
	return 1
}
func luaBitOr(state *lua.State) int {
	state.PushUnsigned(uint(lua.CheckUnsigned(state, 1) | lua.CheckUnsigned(state, 2)))
	return 1
}
func luaBitLShift(state *lua.State) int {
	state.PushUnsigned(uint(uint32(lua.CheckUnsigned(state, 1)) << uint32(lua.CheckUnsigned(state, 2))))
	return 1
}
func luaBitRShift(state *lua.State) int {
	state.PushUnsigned(uint(uint32(lua.CheckUnsigned(state, 1)) >> uint32(lua.CheckUnsigned(state, 2))))
	return 1
}
func luaBitXor(state *lua.State) int {
	state.PushUnsigned(uint(lua.CheckUnsigned(state, 1) ^ lua.CheckUnsigned(state, 2)))
	return 1
}
func luaBitNot(state *lua.State) int {
	state.PushUnsigned(uint(^uint32(lua.CheckUnsigned(state, 1))))
	return 1
}

func objectUint64(object *Object, field string) (uint64, bool) {
	if object == nil {
		return 0, false
	}
	value, ok := object.Fields[field]
	if !ok || value == nil {
		return 0, false
	}
	switch value := value.(type) {
	case UInt64:
		return uint64(value), true
	case Int64:
		if value >= 0 {
			return uint64(value), true
		}
	case uint64:
		return value, true
	case uint32:
		return uint64(value), true
	case uint16:
		return uint64(value), true
	case uint8:
		return uint64(value), true
	case uint:
		return uint64(value), true
	case int:
		if value >= 0 {
			return uint64(value), true
		}
	case int8:
		if value >= 0 {
			return uint64(value), true
		}
	case int16:
		if value >= 0 {
			return uint64(value), true
		}
	case int32:
		if value >= 0 {
			return uint64(value), true
		}
	case int64:
		if value >= 0 {
			return uint64(value), true
		}
	case float64:
		if value >= 0 && value <= math.MaxUint64 && value == math.Trunc(value) {
			return uint64(value), true
		}
	}
	return 0, false
}

func objectUint32(object *Object, field string) (uint32, bool) {
	value, ok := objectUint64(object, field)
	return uint32(value), ok && value <= math.MaxUint32
}

func makeGlobalGUID(high uint16, low, _ uint32) uint64 {
	if low == 0 {
		return 0
	}
	return uint64(low) | uint64(high)<<48
}

func makeMapGUID(high uint16, low, entry uint32) uint64 {
	if low == 0 {
		return 0
	}
	return uint64(low&0x00FFFFFF) | uint64(entry&0x00FFFFFF)<<24 | uint64(high)<<48
}

func guidHasEntry(high uint16) bool {
	switch high {
	case guidHighGameObject, guidHighTransport, guidHighUnit, guidHighPet, guidHighVehicle:
		return true
	default:
		return false
	}
}

func guidLow(guid uint64) uint32 {
	if guidHasEntry(uint16(guid >> 48)) {
		return uint32(guid & 0x00FFFFFF)
	}
	return uint32(guid)
}

func slogLevel(level string) slog.Level {
	switch level {
	case "error":
		return slog.LevelError
	case "debug":
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}
