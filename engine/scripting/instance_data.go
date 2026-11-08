package scripting

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Shopify/go-lua"
)

// Instance-data persistence (ElunaInstanceAI::Save/Load,
// ElunaInstanceAI.cpp:32-152; InstanceScript::SaveToDB,
// Instances/InstanceScript.cpp:60-71): C++ mar_encodes the instance
// data table and base64-encodes it into instance.data. Go cannot read
// a mar_encode blob (no lua-marshal port) and never wrote one, so the
// Go format is its own: a versioned JSON document of key/value entries
// that preserves key types (string vs number) and nested tables,
// base64-encoded the same way. Functions, userdata, threads and cyclic
// tables are rejected with an error: C++ mar_encode handles functions
// and cycles, but instance scripts store numbers/strings/bools/nested
// tables in practice (data.riftState, data.nextPortalMs), and a JSON
// bridge cannot represent a Lua closure. The decode side accepts only
// this format; a C++-produced blob is not readable by it. The blob also
// carries a bossStates section (InstanceScript::WriteSaveDataBossStates,
// Instances/InstanceScript.cpp:471-475, which writes each boss's raw
// state in boss-id order): the section is optional, so blobs written
// before it existed decode with no boss states.

const instanceDataFormatVersion = 1

const instanceDataMaxDepth = 32

type instanceDataBlob struct {
	Version    int                     `json:"v"`
	Entries    []instanceDataEntry     `json:"entries"`
	BossStates []instanceDataBossState `json:"bossStates,omitempty"`
}

// instanceDataBossState is one boss's saved state, in boss-id order:
// C++ writes the raw EncounterState (0 NOT_STARTED .. 5 TO_BE_DECIDED)
// and normalizes IN_PROGRESS/FAIL/SPECIAL to NOT_STARTED on load
// (ReadSaveDataBossStates, InstanceScript.cpp:437-450), never on save.
type instanceDataBossState struct {
	BossID uint32 `json:"b"`
	State  uint8  `json:"s"`
}

type instanceDataEntry struct {
	Key   instanceDataJSONKey   `json:"k"`
	Value instanceDataJSONValue `json:"v"`
}

type instanceDataJSONKey struct {
	Str *string  `json:"s,omitempty"`
	Num *float64 `json:"n,omitempty"`
}

type instanceDataJSONValue struct {
	Kind  string              `json:"t"`
	Bool  bool                `json:"b,omitempty"`
	Num   float64             `json:"n,omitempty"`
	Str   string              `json:"s,omitempty"`
	Table []instanceDataEntry `json:"e,omitempty"`
}

// MarshalInstanceData serializes the (map, instance) data table to the
// base64 blob stored in instance.data, plus the live boss states the
// caller passes (nil when the caller has none; an instance with no data
// table yet marshals as the empty table, matching C++ SaveToDB on a
// fresh shim; a nil runtime reads empty). Boss states are written raw
// and in boss-id order, matching C++ WriteSaveDataBossStates; the
// load-side normalization lives in the caller.
func (r *Runtime) MarshalInstanceData(mapID, instanceID uint32, bossStates map[uint32]uint8) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == nil {
		return "", nil
	}
	obj := &Object{Type: "InstanceData", Fields: map[string]any{"MapId": mapID, "InstanceId": instanceID}}
	blob := instanceDataBlob{Version: instanceDataFormatVersion, BossStates: sortedInstanceDataBossStates(bossStates)}
	if !pushInstanceDataTable(r.state, obj, false) {
		r.state.Pop(1)
	} else {
		entries, err := marshalInstanceDataTable(r.state, 0, nil)
		r.state.Pop(1)
		if err != nil {
			return "", err
		}
		blob.Entries = entries
	}
	raw, err := json.Marshal(blob)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// InstanceDataNumber reads one numeric field of the (map, instance) data
// table, reporting false when the table or field is absent or not a
// number — the Go-side read of the Lua shared instanceData table (the
// ElunaInstanceAI Lua data table), for engine code that must evaluate
// instance-script state without a Lua call frame (e.g. the
// DATA_TYPE_INSTANCE_SCRIPT achievement gate, AchievementMgr.cpp:404-424,
// whose C++ form reads the instance script's members directly). A nil
// runtime reads absent, matching MarshalInstanceData above.
func (r *Runtime) InstanceDataNumber(mapID, instanceID uint32, key string) (float64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == nil {
		return 0, false
	}
	obj := &Object{Type: "InstanceData", Fields: map[string]any{"MapId": mapID, "InstanceId": instanceID}}
	if !pushInstanceDataTable(r.state, obj, false) {
		r.state.Pop(1)
		return 0, false
	}
	defer r.state.Pop(2)
	r.state.PushString(key)
	r.state.RawGet(-2)
	if !r.state.IsNumber(-1) {
		return 0, false
	}
	v, ok := r.state.ToNumber(-1)
	return v, ok
}

// UnmarshalInstanceData replaces the (map, instance) data table with
// the decoded blob and returns the boss states the blob carried (nil
// when the blob has no bossStates section, as with blobs written before
// the section existed): C++ Load swaps in the decoded table wholesale
// (CreateInstanceData), so stale keys from a previous load do not
// survive. The table keeps its registry identity — scripts only ever
// hold the InstanceData userdata handle, never the raw table. The
// caller applies the boss states itself, normalizing them first
// (InstanceScript::ReadSaveDataBossStates).
func (r *Runtime) UnmarshalInstanceData(mapID, instanceID uint32, encoded string) (map[uint32]uint8, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == nil {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var blob instanceDataBlob
	if err := json.Unmarshal(raw, &blob); err != nil {
		return nil, err
	}
	if blob.Version != instanceDataFormatVersion {
		return nil, fmt.Errorf("unsupported instance data version %d", blob.Version)
	}
	obj := &Object{Type: "InstanceData", Fields: map[string]any{"MapId": mapID, "InstanceId": instanceID}}
	if !pushInstanceDataTable(r.state, obj, true) {
		r.state.Pop(1)
		return nil, fmt.Errorf("instance data table unavailable")
	}
	clearInstanceDataTable(r.state)
	for _, entry := range blob.Entries {
		pushInstanceDataKey(r.state, entry.Key)
		pushInstanceDataValue(r.state, entry.Value)
		r.state.RawSet(-3)
	}
	r.state.Pop(1)
	return decodedInstanceDataBossStates(blob.BossStates), nil
}

// sortedInstanceDataBossStates flattens the live boss map into
// boss-id order, the order C++ writes them in.
func sortedInstanceDataBossStates(bossStates map[uint32]uint8) []instanceDataBossState {
	if len(bossStates) == 0 {
		return nil
	}
	ids := make([]uint32, 0, len(bossStates))
	for id := range bossStates {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	states := make([]instanceDataBossState, 0, len(ids))
	for _, id := range ids {
		states = append(states, instanceDataBossState{BossID: id, State: bossStates[id]})
	}
	return states
}

// decodedInstanceDataBossStates flattens the blob section back into a
// boss map; later entries win, matching the C++ read loop, which sets
// each boss id in turn.
func decodedInstanceDataBossStates(states []instanceDataBossState) map[uint32]uint8 {
	if len(states) == 0 {
		return nil
	}
	bossStates := make(map[uint32]uint8, len(states))
	for _, state := range states {
		bossStates[state.BossID] = state.State
	}
	return bossStates
}

// marshalInstanceDataTable converts the table on top of the stack into
// entries, leaving the table on top. ancestors holds the absolute stack
// indices of the tables above this one, so a nested table that RawEquals
// any ancestor is a cycle. Depth is capped: a deeper nest is a runaway
// table, not instance data.
func marshalInstanceDataTable(state *lua.State, depth int, ancestors []int) ([]instanceDataEntry, error) {
	if depth > instanceDataMaxDepth {
		return nil, fmt.Errorf("instance data table too deep")
	}
	tableIndex := state.Top()
	state.PushNil()
	entries := []instanceDataEntry{}
	for state.Next(-2) {
		key, err := marshalInstanceDataKey(state, -2)
		if err != nil {
			state.Pop(2)
			return nil, err
		}
		value, err := marshalInstanceDataValue(state, -1, depth, append(ancestors, tableIndex))
		if err != nil {
			state.Pop(2)
			return nil, err
		}
		entries = append(entries, instanceDataEntry{Key: key, Value: value})
		state.Pop(1)
	}
	return entries, nil
}

func marshalInstanceDataKey(state *lua.State, index int) (instanceDataJSONKey, error) {
	// TypeOf, not IsString/IsNumber: go-lua's IsString is true for
	// numbers and IsNumber is true for numeric strings, which would
	// collapse the string/number key distinction the blob preserves.
	switch state.TypeOf(index) {
	case lua.TypeString:
		value, _ := state.ToString(index)
		return instanceDataJSONKey{Str: &value}, nil
	case lua.TypeNumber:
		value, _ := state.ToNumber(index)
		return instanceDataJSONKey{Num: &value}, nil
	default:
		return instanceDataJSONKey{}, fmt.Errorf("unsupported instance data key type %s", state.TypeOf(index))
	}
}

func marshalInstanceDataValue(state *lua.State, index int, depth int, ancestors []int) (instanceDataJSONValue, error) {
	switch state.TypeOf(index) {
	case lua.TypeNil:
		return instanceDataJSONValue{Kind: "nil"}, nil
	case lua.TypeBoolean:
		return instanceDataJSONValue{Kind: "bool", Bool: state.ToBoolean(index)}, nil
	case lua.TypeNumber:
		value, _ := state.ToNumber(index)
		return instanceDataJSONValue{Kind: "num", Num: value}, nil
	case lua.TypeString:
		value, _ := state.ToString(index)
		return instanceDataJSONValue{Kind: "str", Str: value}, nil
	case lua.TypeTable:
		for _, ancestor := range ancestors {
			if state.RawEqual(index, ancestor) {
				return instanceDataJSONValue{}, fmt.Errorf("cyclic instance data table")
			}
		}
		entries, err := marshalInstanceDataTable(state, depth+1, ancestors)
		if err != nil {
			return instanceDataJSONValue{}, err
		}
		return instanceDataJSONValue{Kind: "table", Table: entries}, nil
	default:
		return instanceDataJSONValue{}, fmt.Errorf("unsupported instance data value type %s", state.TypeOf(index))
	}
}

// clearInstanceDataTable drops every key of the table on top of the
// stack, leaving the table on top. The keys are collected first and
// deleted after the iteration finishes: deleting the current key
// inside the loop makes the next Next call fail with "invalid key to
// 'next'" in this go-lua, because the deleted key is no longer in the
// table's hash — the in-place delete-during-iteration idiom panics on
// any non-empty table.
func clearInstanceDataTable(state *lua.State) {
	tableIndex := state.Top()
	state.PushNil()
	keyCount := 0
	for state.Next(tableIndex) {
		state.Pop(1)
		state.PushValue(-1)
		keyCount++
	}
	for i := 0; i < keyCount; i++ {
		state.PushValue(tableIndex + 1 + i)
		state.PushNil()
		state.RawSet(tableIndex)
	}
	state.SetTop(tableIndex)
}

func pushInstanceDataKey(state *lua.State, key instanceDataJSONKey) {
	if key.Str != nil {
		state.PushString(*key.Str)
		return
	}
	if key.Num != nil {
		state.PushNumber(*key.Num)
		return
	}
	state.PushNil()
}

func pushInstanceDataValue(state *lua.State, value instanceDataJSONValue) {
	switch value.Kind {
	case "bool":
		state.PushBoolean(value.Bool)
	case "num":
		state.PushNumber(value.Num)
	case "str":
		state.PushString(value.Str)
	case "table":
		state.NewTable()
		for _, entry := range value.Table {
			pushInstanceDataKey(state, entry.Key)
			pushInstanceDataValue(state, entry.Value)
			state.RawSet(-3)
		}
	default:
		state.PushNil()
	}
}
