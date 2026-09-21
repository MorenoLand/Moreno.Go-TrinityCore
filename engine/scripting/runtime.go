package scripting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Shopify/go-lua"
)

const (
	PlayerEventLogin      = 3
	PlayerEventLogout     = 4
	PlayerEventChat       = 18
	PlayerEventUpdateZone = 27
	PlayerEventCommand    = 42
)

type Hook struct {
	Kind  string
	Event int
}

type ObjectMethod func(context.Context, []any) ([]any, error)

type Object struct {
	Type    string
	Methods map[string]ObjectMethod
	Fields  map[string]any
}

type Config struct {
	Enabled        bool
	ScriptPath     string
	CoreName       string
	CoreVersion    string
	RealmID        uint32
	CoreExpansion  uint32
	PlayerProvider func() []*Object
	AuthDatabase   *sql.DB
	CharacterDB    *sql.DB
	WorldDatabase  *sql.DB
	Logger         *slog.Logger
}

type Runtime struct {
	config       Config
	mu           sync.Mutex
	state        *lua.State
	nextRef      int
	nextHook     int
	hooks        []registeredHook
	cancelled    map[int]struct{}
	timers       []timer
	loadedFiles  []string
	loadFailures []error
}

type registeredHook struct {
	Hook
	id    int
	ref   int
	shots int
}

type timer struct {
	id       int
	ref      int
	minDelay int64
	maxDelay int64
	delay    int64
	repeats  int
	elapsed  int64
}

func NewRuntime(c Config) *Runtime {
	return &Runtime{config: c, nextRef: 3, nextHook: 1, cancelled: make(map[int]struct{})}
}

func (r *Runtime) Load(ctx context.Context) error {
	if !r.config.Enabled || strings.TrimSpace(r.config.ScriptPath) == "" {
		return nil
	}
	if _, err := os.Stat(r.config.ScriptPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initializeLocked()
	files := make([]string, 0)
	err := filepath.WalkDir(r.config.ScriptPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".lua" || extension == ".ext" {
			if strings.HasSuffix(strings.ToLower(path), string(filepath.Separator)+"stacktraceplus"+string(filepath.Separator)+"stacktraceplus.ext") {
				return nil
			}
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool {
		iExt := strings.ToLower(filepath.Ext(files[i]))
		jExt := strings.ToLower(filepath.Ext(files[j]))
		if iExt != jExt {
			return iExt == ".ext"
		}
		return files[i] < files[j]
	})
	for _, path := range files {
		r.state.SetTop(0)
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			r.state.SetTop(0)
			r.loadFailures = append(r.loadFailures, fmt.Errorf("%s: %w", path, readErr))
			if r.config.Logger != nil {
				r.config.Logger.Error("lua script failed", "path", path, "error", readErr)
			}
			continue
		}
		loadErr := r.state.Load(strings.NewReader(string(source)), "@"+path, "")
		if loadErr == nil {
			loadErr = r.state.ProtectedCall(0, lua.MultipleReturns, 0)
		}
		if loadErr != nil {
			r.state.SetTop(0)
			r.loadFailures = append(r.loadFailures, fmt.Errorf("%s: %w", path, loadErr))
			if r.config.Logger != nil {
				r.config.Logger.Error("lua script failed", "path", path, "error", loadErr)
			}
			continue
		}
		r.state.SetTop(0)
		r.loadedFiles = append(r.loadedFiles, path)
	}
	_ = ctx
	return nil
}

func (r *Runtime) LoadString(source string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initializeLocked()
	return lua.DoString(r.state, source)
}

func (r *Runtime) LoadedFiles() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.loadedFiles...)
}

func (r *Runtime) LoadFailures() []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]error(nil), r.loadFailures...)
}

func (r *Runtime) Hooks() []Hook {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Hook, 0, len(r.hooks))
	for _, hook := range r.hooks {
		result = append(result, hook.Hook)
	}
	return result
}

func (r *Runtime) Trigger(ctx context.Context, kind string, event int, args ...any) ([]any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == nil {
		return nil, nil
	}
	result := make([]any, 0)
	initial := append([]registeredHook(nil), r.hooks...)
	initialIDs := make(map[int]struct{}, len(initial))
	for _, hook := range initial {
		initialIDs[hook.id] = struct{}{}
	}
	remaining := make([]registeredHook, 0, len(initial))
	for _, hook := range initial {
		if _, cancelled := r.cancelled[hook.id]; cancelled {
			continue
		}
		if hook.Kind != kind || hook.Event != event {
			remaining = append(remaining, hook)
			continue
		}
		remove := hook.shots == 1
		if hook.shots > 1 {
			hook.shots--
		}
		r.state.SetTop(0)
		r.state.RawGetInt(lua.RegistryIndex, hook.ref)
		for _, arg := range args {
			if err := pushValue(r.state, arg); err != nil {
				return result, err
			}
		}
		if err := r.state.ProtectedCall(len(args), 1, 0); err != nil {
			if r.config.Logger != nil {
				r.config.Logger.Error("lua hook failed", "kind", kind, "event", event, "error", err)
			}
			if !remove {
				if _, cancelled := r.cancelled[hook.id]; !cancelled {
					remaining = append(remaining, hook)
				}
			}
			continue
		}
		if r.state.Top() != 0 {
			result = append(result, luaValue(r.state, -1))
		}
		if !remove {
			if _, cancelled := r.cancelled[hook.id]; !cancelled {
				remaining = append(remaining, hook)
			}
		}
	}
	for _, hook := range r.hooks {
		if _, existed := initialIDs[hook.id]; existed {
			continue
		}
		if _, cancelled := r.cancelled[hook.id]; !cancelled {
			remaining = append(remaining, hook)
		}
	}
	r.hooks = remaining
	_ = ctx
	return result, nil
}

func (r *Runtime) TriggerPlayerEvent(ctx context.Context, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "player", event, args...)
}

func (r *Runtime) TriggerServerEvent(ctx context.Context, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "server", event, append([]any{event}, args...)...)
}

func (r *Runtime) TriggerPacketEvent(ctx context.Context, opcode, event int, args ...any) ([]any, error) {
	if !r.mu.TryLock() {
		return nil, nil
	}
	r.mu.Unlock()
	return r.Trigger(ctx, "packet:"+strconv.Itoa(opcode), event, append([]any{event}, args...)...)
}

func (r *Runtime) Tick(ctx context.Context, elapsed int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == nil || elapsed < 0 {
		return nil
	}
	remaining := r.timers[:0]
	for _, current := range r.timers {
		current.elapsed += elapsed
		removeCurrent := false
		for current.elapsed >= current.delay {
			current.elapsed -= current.delay
			callbackDelay := current.delay
			remove := current.repeats == 1
			remainingRepeats := current.repeats
			if current.repeats > 1 {
				current.repeats--
			}
			r.state.SetTop(0)
			r.state.RawGetInt(lua.RegistryIndex, current.ref)
			r.state.PushInteger(current.id)
			r.state.PushInteger(int(callbackDelay))
			r.state.PushInteger(remainingRepeats)
			if err := r.state.ProtectedCall(3, 0, 0); err != nil && r.config.Logger != nil {
				r.config.Logger.Error("lua timer failed", "id", current.id, "error", err)
			}
			if remove {
				removeCurrent = true
				break
			}
			current.delay = timerDelay(current.minDelay, current.maxDelay)
		}
		if !removeCurrent {
			remaining = append(remaining, current)
		}
	}
	r.timers = remaining
	_ = ctx
	return nil
}

func (r *Runtime) initializeLocked() {
	if r.state != nil {
		return
	}
	r.state = lua.NewState()
	lua.OpenLibraries(r.state)
	installLuaStringCompatibility(r.state)
	for _, name := range []string{"Map", "Player", "Creature", "GameObject"} {
		r.state.NewTable()
		r.state.SetGlobal(name)
	}
	r.state.Register("RegisterPlayerEvent", r.registerPlayerEvent)
	r.state.Register("RegisterGuildEvent", r.registerGuildEvent)
	r.state.Register("RegisterGroupEvent", r.registerGroupEvent)
	r.state.Register("RegisterCreatureEvent", r.registerCreatureEvent)
	r.state.Register("RegisterUniqueCreatureEvent", r.registerUniqueCreatureEvent)
	r.state.Register("RegisterCreatureGossipEvent", r.registerCreatureGossipEvent)
	r.state.Register("RegisterGameObjectEvent", r.registerGameObjectEvent)
	r.state.Register("RegisterGameObjectGossipEvent", r.registerGameObjectGossipEvent)
	r.state.Register("RegisterItemEvent", r.registerItemEvent)
	r.state.Register("RegisterItemGossipEvent", r.registerItemGossipEvent)
	r.state.Register("RegisterPlayerGossipEvent", r.registerPlayerGossipEvent)
	r.state.Register("RegisterBGEvent", r.registerBGEvent)
	r.state.Register("RegisterPacketEvent", r.registerPacketEvent)
	r.state.Register("RegisterMapEvent", r.registerMapEvent)
	r.state.Register("RegisterInstanceEvent", r.registerInstanceEvent)
	r.state.Register("RegisterServerEvent", r.registerServerEvent)
	r.state.Register("RegisterGlobalEvent", r.registerServerEvent)
	r.state.Register("ClearBattleGroundEvents", r.clearBattleGroundEvents)
	r.state.Register("ClearCreatureEvents", r.clearCreatureEvents)
	r.state.Register("ClearUniqueCreatureEvents", r.clearUniqueCreatureEvents)
	r.state.Register("ClearCreatureGossipEvents", r.clearCreatureGossipEvents)
	r.state.Register("ClearGameObjectEvents", r.clearGameObjectEvents)
	r.state.Register("ClearGameObjectGossipEvents", r.clearGameObjectGossipEvents)
	r.state.Register("ClearGroupEvents", r.clearGroupEvents)
	r.state.Register("ClearGuildEvents", r.clearGuildEvents)
	r.state.Register("ClearItemEvents", r.clearItemEvents)
	r.state.Register("ClearItemGossipEvents", r.clearItemGossipEvents)
	r.state.Register("ClearPacketEvents", r.clearPacketEvents)
	r.state.Register("ClearPlayerEvents", r.clearPlayerEvents)
	r.state.Register("ClearPlayerGossipEvents", r.clearPlayerGossipEvents)
	r.state.Register("ClearServerEvents", r.clearServerEvents)
	r.state.Register("ClearMapEvents", r.clearMapEvents)
	r.state.Register("ClearInstanceEvents", r.clearInstanceEvents)
	r.state.Register("GetCoreExpansion", r.getCoreExpansion)
	r.state.Register("GetLuaEngine", r.getLuaEngine)
	r.state.Register("GetCoreName", r.getCoreName)
	r.state.Register("GetRealmID", r.getRealmID)
	r.state.Register("GetCoreVersion", r.getCoreVersion)
	r.state.Register("GetGameTime", r.getGameTime)
	r.state.Register("GetQuest", r.getQuest)
	r.state.Register("GetGuildByName", r.getGuildByName)
	r.state.Register("GetGuildByLeaderGUID", r.getGuildByLeaderGUID)
	r.state.Register("GetPlayersInWorld", r.getPlayersInWorld)
	r.state.Register("GetPlayerByGUID", r.getPlayerByGUID)
	r.state.Register("GetPlayerByName", r.getPlayerByName)
	r.state.Register("GetPlayerCount", r.getPlayerCount)
	r.state.Register("GetPlayerGUID", r.getPlayerGUID)
	r.state.Register("GetItemGUID", r.getItemGUID)
	r.state.Register("GetObjectGUID", r.getObjectGUID)
	r.state.Register("GetUnitGUID", r.getUnitGUID)
	r.state.Register("GetGUIDLow", r.getGUIDLow)
	r.state.Register("GetGUIDType", r.getGUIDType)
	r.state.Register("GetGUIDEntry", r.getGUIDEntry)
	r.state.Register("GetCurrTime", r.getCurrTime)
	r.state.Register("GetTimeDiff", r.getTimeDiff)
	r.state.Register("CreateInt64", r.createInt64)
	r.state.Register("CreateUint64", r.createUint64)
	r.state.Register("CreatePacket", r.createPacket)
	r.state.Register("bit_and", luaBitAnd)
	r.state.Register("bit_or", luaBitOr)
	r.state.Register("bit_lshift", luaBitLShift)
	r.state.Register("bit_rshift", luaBitRShift)
	r.state.Register("bit_xor", luaBitXor)
	r.state.Register("bit_not", luaBitNot)
	r.state.Register("PrintInfo", r.printInfo)
	r.state.Register("PrintError", r.printError)
	r.state.Register("PrintDebug", r.printDebug)
	r.state.Register("CreateLuaEvent", r.createLuaEvent)
	r.state.Register("RemoveEventById", r.removeEvent)
	r.state.Register("RemoveEvents", r.removeEvents)
	r.state.Register("CharDBQuery", r.charDBQuery)
	r.state.Register("WorldDBQuery", r.worldDBQuery)
	r.state.Register("AuthDBQuery", r.authDBQuery)
	r.state.Register("CharDBExecute", r.charDBExecute)
	r.state.Register("WorldDBExecute", r.worldDBExecute)
	r.state.Register("AuthDBExecute", r.authDBExecute)
	lua.NewMetaTable(r.state, queryMetaTable)
	lua.SetFunctions(r.state, []lua.RegistryFunction{{Name: "__index", Function: queryIndex}}, 0)
	r.state.Pop(1)
	lua.NewMetaTable(r.state, objectMetaTable)
	lua.SetFunctions(r.state, []lua.RegistryFunction{{Name: "__index", Function: objectIndex}, {Name: "__eq", Function: objectEqual}}, 0)
	r.state.Pop(1)
	installUInt64MetaTable(r.state)
	installPacketMetaTable(r.state)
	setPackagePath(r.state, r.config.ScriptPath)
}

func (r *Runtime) getCoreExpansion(state *lua.State) int {
	state.PushUnsigned(uint(r.config.CoreExpansion))
	return 1
}

func (r *Runtime) getPlayersInWorld(state *lua.State) int {
	team := uint32(2)
	if state.Top() >= 1 && !state.IsNil(1) {
		team = uint32(lua.CheckUnsigned(state, 1))
	}
	onlyGM := state.Top() >= 2 && !state.IsNil(2) && state.ToBoolean(2)
	state.NewTable()
	if r.config.PlayerProvider == nil {
		return 1
	}
	index := 0
	for _, player := range r.config.PlayerProvider() {
		if player == nil {
			continue
		}
		if team != 2 {
			playerTeam, ok := objectUint32(player, "Team")
			if !ok || playerTeam != team {
				continue
			}
		}
		if onlyGM {
			isGM, ok := player.Fields["IsGM"].(bool)
			if !ok || !isGM {
				continue
			}
		}
		if err := pushValue(state, player); err != nil {
			continue
		}
		index++
		state.RawSetInt(-2, index)
	}
	return 1
}

func (r *Runtime) getPlayerByName(state *lua.State) int {
	name := lua.CheckString(state, 1)
	if r.config.PlayerProvider != nil {
		for _, player := range r.config.PlayerProvider() {
			if player == nil {
				continue
			}
			if value, ok := player.Fields["Name"].(string); ok && strings.EqualFold(value, name) {
				PushObject(state, player)
				return 1
			}
		}
	}
	state.PushNil()
	return 1
}

func (r *Runtime) SetPlayerProvider(provider func() []*Object) {
	r.mu.Lock()
	r.config.PlayerProvider = provider
	r.mu.Unlock()
}

func setPackagePath(state *lua.State, scriptPath string) {
	state.Global("package")
	if state.IsNil(-1) {
		state.Pop(1)
		return
	}
	state.PushString(filepath.Join(scriptPath, "?.lua") + ";" + filepath.Join(scriptPath, "?.ext") + ";" + filepath.Join(scriptPath, "extensions", "?.ext") + ";" + filepath.Join(scriptPath, "extensions", "?", "?.ext"))
	state.SetField(-2, "path")
	state.Pop(1)
}

func (r *Runtime) registerPlayerEvent(state *lua.State) int {
	return r.registerHook(state, "player", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerServerEvent(state *lua.State) int {
	return r.registerHook(state, "server", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerCreatureGossipEvent(state *lua.State) int {
	return r.registerEntryHook(state, "creature_gossip", 1, 2, 3, 4)
}

func (r *Runtime) registerPlayerGossipEvent(state *lua.State) int {
	return r.registerEntryHook(state, "player_gossip", 1, 2, 3, 4)
}

func (r *Runtime) registerGuildEvent(state *lua.State) int {
	return r.registerHook(state, "guild", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerGroupEvent(state *lua.State) int {
	return r.registerHook(state, "group", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerBGEvent(state *lua.State) int {
	return r.registerHook(state, "bg", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerCreatureEvent(state *lua.State) int {
	return r.registerEntryHook(state, "creature", 1, 2, 3, 4)
}

func (r *Runtime) registerUniqueCreatureEvent(state *lua.State) int {
	guid := checkLuaUint64(state, 1)
	instanceID := lua.CheckUnsigned(state, 2)
	event := lua.CheckInteger(state, 3)
	return r.registerHook(state, "creature_unique:"+strconv.FormatUint(guid, 10)+":"+strconv.FormatUint(uint64(instanceID), 10), event, 4, 5)
}

func (r *Runtime) registerGameObjectEvent(state *lua.State) int {
	return r.registerEntryHook(state, "gameobject", 1, 2, 3, 4)
}

func (r *Runtime) registerGameObjectGossipEvent(state *lua.State) int {
	return r.registerEntryHook(state, "gameobject_gossip", 1, 2, 3, 4)
}

func (r *Runtime) registerItemEvent(state *lua.State) int {
	return r.registerEntryHook(state, "item", 1, 2, 3, 4)
}

func (r *Runtime) registerItemGossipEvent(state *lua.State) int {
	return r.registerEntryHook(state, "item_gossip", 1, 2, 3, 4)
}

func (r *Runtime) registerPacketEvent(state *lua.State) int {
	return r.registerEntryHook(state, "packet", 1, 2, 3, 4)
}

func (r *Runtime) registerMapEvent(state *lua.State) int {
	return r.registerEntryHook(state, "map", 1, 2, 3, 4)
}

func (r *Runtime) registerInstanceEvent(state *lua.State) int {
	return r.registerEntryHook(state, "instance", 1, 2, 3, 4)
}

func (r *Runtime) registerEntryHook(state *lua.State, kind string, entryIndex, eventIndex, functionIndex, shotsIndex int) int {
	entry := lua.CheckUnsigned(state, entryIndex)
	event := lua.CheckInteger(state, eventIndex)
	return r.registerHook(state, kind+":"+strconv.FormatUint(uint64(entry), 10), event, functionIndex, shotsIndex)
}

func (r *Runtime) registerHook(state *lua.State, kind string, event, functionIndex, shotsIndex int) int {
	if !state.IsFunction(functionIndex) {
		lua.ArgumentError(state, functionIndex, "function expected")
	}
	shots := 0
	if state.Top() >= shotsIndex && !state.IsNil(shotsIndex) {
		shots = lua.CheckInteger(state, shotsIndex)
		if shots < 0 {
			lua.ArgumentError(state, shotsIndex, "non-negative shots expected")
		}
	}
	ref := r.storeFunction(state, functionIndex)
	id := r.nextHook
	r.nextHook++
	r.hooks = append(r.hooks, registeredHook{Hook: Hook{Kind: kind, Event: event}, id: id, ref: ref, shots: shots})
	state.PushGoFunction(func(_ *lua.State) int {
		r.cancelHook(id)
		return 0
	})
	return 1
}

func (r *Runtime) clearBattleGroundEvents(state *lua.State) int {
	return r.clearGlobalEvents(state, "bg")
}
func (r *Runtime) clearGroupEvents(state *lua.State) int { return r.clearGlobalEvents(state, "group") }
func (r *Runtime) clearGuildEvents(state *lua.State) int { return r.clearGlobalEvents(state, "guild") }
func (r *Runtime) clearPlayerEvents(state *lua.State) int {
	return r.clearGlobalEvents(state, "player")
}
func (r *Runtime) clearServerEvents(state *lua.State) int {
	return r.clearGlobalEvents(state, "server")
}

func (r *Runtime) clearCreatureEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "creature")
}

func (r *Runtime) clearCreatureGossipEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "creature_gossip")
}

func (r *Runtime) clearGameObjectEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "gameobject")
}

func (r *Runtime) clearGameObjectGossipEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "gameobject_gossip")
}

func (r *Runtime) clearItemEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "item")
}

func (r *Runtime) clearItemGossipEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "item_gossip")
}

func (r *Runtime) clearPacketEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "packet")
}

func (r *Runtime) clearPlayerGossipEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "player_gossip")
}

func (r *Runtime) clearMapEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "map")
}

func (r *Runtime) clearInstanceEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "instance")
}

func (r *Runtime) clearUniqueCreatureEvents(state *lua.State) int {
	guid := checkLuaUint64(state, 1)
	instanceID := lua.CheckUnsigned(state, 2)
	prefix := "creature_unique:" + strconv.FormatUint(guid, 10) + ":" + strconv.FormatUint(uint64(instanceID), 10)
	event := optionalEvent(state, 3)
	r.clearHooks(func(hook registeredHook) bool { return hook.Kind == prefix && (event < 0 || hook.Event == event) })
	return 0
}

func (r *Runtime) clearGlobalEvents(state *lua.State, kind string) int {
	event := optionalEvent(state, 1)
	r.clearHooks(func(hook registeredHook) bool { return hook.Kind == kind && (event < 0 || hook.Event == event) })
	return 0
}

func (r *Runtime) clearEntryEvents(state *lua.State, kind string) int {
	entry := lua.CheckUnsigned(state, 1)
	prefix := kind + ":" + strconv.FormatUint(uint64(entry), 10)
	event := optionalEvent(state, 2)
	r.clearHooks(func(hook registeredHook) bool { return hook.Kind == prefix && (event < 0 || hook.Event == event) })
	return 0
}

func optionalEvent(state *lua.State, index int) int {
	if state.Top() < index || state.IsNil(index) {
		return -1
	}
	return lua.CheckInteger(state, index)
}

func (r *Runtime) clearHooks(predicate func(registeredHook) bool) {
	remaining := r.hooks[:0]
	for _, hook := range r.hooks {
		if predicate(hook) {
			r.cancelled[hook.id] = struct{}{}
			continue
		}
		remaining = append(remaining, hook)
	}
	r.hooks = remaining
}

func (r *Runtime) cancelHook(id int) {
	r.cancelled[id] = struct{}{}
	remaining := r.hooks[:0]
	for _, hook := range r.hooks {
		if hook.id != id {
			remaining = append(remaining, hook)
		}
	}
	r.hooks = remaining
}

func (r *Runtime) storeFunction(state *lua.State, index int) int {
	ref := r.nextRef
	r.nextRef++
	state.PushValue(index)
	state.RawSetInt(lua.RegistryIndex, ref)
	return ref
}

func (r *Runtime) createLuaEvent(state *lua.State) int {
	if !state.IsFunction(1) {
		lua.ArgumentError(state, 1, "function expected")
	}
	var minDelay, maxDelay int64
	if state.IsTable(2) {
		state.RawGetInt(2, 1)
		minDelay = int64(lua.CheckInteger(state, -1))
		state.Pop(1)
		state.RawGetInt(2, 2)
		maxDelay = int64(lua.CheckInteger(state, -1))
		state.Pop(1)
	} else {
		minDelay = int64(lua.CheckInteger(state, 2))
		maxDelay = minDelay
	}
	if minDelay < 1 || maxDelay < minDelay {
		lua.ArgumentError(state, 2, "invalid delay range")
	}
	repeats := 1
	if state.Top() >= 3 && state.IsNumber(3) {
		repeats = lua.CheckInteger(state, 3)
	}
	id := r.nextRef
	r.timers = append(r.timers, timer{id: id, ref: r.storeFunction(state, 1), minDelay: minDelay, maxDelay: maxDelay, delay: timerDelay(minDelay, maxDelay), repeats: repeats})
	state.PushInteger(id)
	return 1
}

func timerDelay(minDelay, maxDelay int64) int64 {
	if maxDelay <= minDelay {
		return minDelay
	}
	return minDelay + rand.Int63n(maxDelay-minDelay+1)
}

func (r *Runtime) removeEvent(state *lua.State) int {
	id := lua.CheckInteger(state, 1)
	for index, current := range r.timers {
		if current.id == id {
			r.timers = append(r.timers[:index], r.timers[index+1:]...)
			break
		}
	}
	return 0
}

func (r *Runtime) removeEvents(state *lua.State) int {
	r.timers = nil
	return 0
}

func (r *Runtime) charDBQuery(state *lua.State) int  { return r.dbQuery(state, r.config.CharacterDB) }
func (r *Runtime) worldDBQuery(state *lua.State) int { return r.dbQuery(state, r.config.WorldDatabase) }
func (r *Runtime) authDBQuery(state *lua.State) int  { return r.dbQuery(state, r.config.AuthDatabase) }
func (r *Runtime) charDBExecute(state *lua.State) int {
	return r.dbExecute(state, r.config.CharacterDB)
}
func (r *Runtime) worldDBExecute(state *lua.State) int {
	return r.dbExecute(state, r.config.WorldDatabase)
}
func (r *Runtime) authDBExecute(state *lua.State) int {
	return r.dbExecute(state, r.config.AuthDatabase)
}

func (r *Runtime) dbQuery(state *lua.State, db *sql.DB) int {
	sqlText := lua.CheckString(state, 1)
	query, err := queryDatabase(db, sqlText)
	if err != nil {
		if r.config.Logger != nil {
			r.config.Logger.Error("lua database query failed", "error", err)
		}
		state.PushNil()
		return 1
	}
	if query == nil {
		state.PushNil()
		return 1
	}
	pushQuery(state, query)
	return 1
}

func (r *Runtime) dbExecute(state *lua.State, db *sql.DB) int {
	sqlText := lua.CheckString(state, 1)
	if err := executeDatabase(db, sqlText); err != nil && r.config.Logger != nil {
		r.config.Logger.Error("lua database execute failed", "error", err)
	}
	return 0
}
