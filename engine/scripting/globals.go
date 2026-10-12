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
	var id, level, minLevel, flags, nextID, chainNextID, prevID, questType int64
	var title string
	err := r.config.WorldDatabase.QueryRow("SELECT ID, COALESCE(LogTitle, ''), COALESCE(QuestLevel, 0), COALESCE(MinLevel, 0), COALESCE(Flags, 0), COALESCE(RewardNextQuest, 0), COALESCE(NextQuestID, 0), COALESCE(PrevQuestId, 0), COALESCE(Type, 0) FROM quest_template WHERE ID = ?", uint32(questID)).Scan(&id, &title, &level, &minLevel, &flags, &chainNextID, &nextID, &prevID, &questType)
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
	// GetNextQuestId is quest_template.NextQuestID (_nextQuestId,
	// QuestDef.cpp:179) while GetNextQuestInChain is RewardNextQuest
	// (_rewardNextQuest, QuestDef.h:253) — Eluna splits them
	// (QuestMethods.h); Go previously returned RewardNextQuest from
	// GetNextQuestId.
	methods["GetNextQuestId"] = func(context.Context, []any) ([]any, error) { return []any{int32(nextID)}, nil }
	methods["GetNextQuestInChain"] = func(context.Context, []any) ([]any, error) { return []any{int32(chainNextID)}, nil }
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
	return r.pushGuildLookup(state, "SELECT guildid, name, leaderguid, motd, info FROM guild WHERE name = ? LIMIT 1", lua.CheckString(state, 1))
}

func (r *Runtime) getGuildByLeaderGUID(state *lua.State) int {
	return r.pushGuildLookup(state, "SELECT guildid, name, leaderguid, motd, info FROM guild WHERE leaderguid = ? LIMIT 1", checkLuaUint64(state, 1))
}

// GuildSnapshot is the live guild row behind a Lua Guild object: the fields
// Eluna's GuildMethods.h reads from the live Guild*.
type GuildSnapshot struct {
	Name        string
	LeaderGUID  uint64
	MemberCount uint32
	MOTD        string
	Info        string
}

// GuildLive carries the live-data callbacks behind a Lua Guild object. Load
// re-reads the guild row (Eluna methods read the live Guild*, not a
// push-time copy); Players enumerates the online player objects for the
// GetLeader/GetMembers filters. Either may be nil.
type GuildLive struct {
	Load    func() (GuildSnapshot, bool)
	Players func() []*Object
}

// playerObjectByGUID finds an online player object by full GUID, the Go
// model of eObjectAccessor()FindPlayer for the Eluna guild/group filters.
func playerObjectByGUID(players func() []*Object, guid uint64) *Object {
	if players == nil {
		return nil
	}
	for _, player := range players() {
		if value, ok := objectUint64(player, "GUID"); ok && value == guid {
			return player
		}
	}
	return nil
}

// objectGuildID reads a player object's guild through its GetGuildId method,
// the accessor luaPlayer exposes (world lua_player.go).
func objectGuildID(ctx context.Context, object *Object) (uint32, bool) {
	if object == nil {
		return 0, false
	}
	method, ok := object.Methods["GetGuildId"]
	if !ok || method == nil {
		return 0, false
	}
	values, err := method(ctx, nil)
	if err != nil || len(values) == 0 {
		return 0, false
	}
	number, ok := numericValue(values[0])
	if !ok || number < 0 || number > math.MaxUint32 || number != math.Trunc(number) {
		return 0, false
	}
	return uint32(number), true
}

// objectArgumentUint64 is the uint64 twin of objectArgumentUint32.
func objectArgumentUint64(args []any, index int) (uint64, error) {
	if index >= len(args) {
		return 0, fmt.Errorf("argument %d is required", index+1)
	}
	number, ok := numericValue(args[index])
	if !ok || number < 0 || number > math.MaxUint64 || number != math.Trunc(number) {
		return 0, fmt.Errorf("argument %d must be uint64", index+1)
	}
	return uint64(number), nil
}

// NewGuildObject builds the Lua Guild object surface: the ID, Name,
// LeaderGUID, MemberCount, MOTD and Info fields plus the GetId/GetName/
// GetLeaderGUID/GetMemberCount/GetMOTD/GetInfo/GetLeader/GetMembers methods.
// It is the Go model of Eluna's Push(guild) (LuaEngine/GuildHooks.cpp via
// ElunaTemplate), shared by the GetGuildBy* globals and the world engine's
// guild event hooks. The mutating surface (SetLeader/SetBankTabText/
// SendPacket/SendPacketToRanked/Disband/AddMember/DeleteMember/
// SetMemberRank, GuildMethods.h) has no script-safe live path yet and stays
// unbridged.
func NewGuildObject(guildID uint32, live GuildLive) *Object {
	load := live.Load
	if load == nil {
		load = func() (GuildSnapshot, bool) { return GuildSnapshot{}, false }
	}
	snap, _ := load()
	methods := map[string]ObjectMethod{
		"GetId": func(context.Context, []any) ([]any, error) { return []any{uint32(guildID)}, nil },
		"GetName": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.Name}, nil
			}
			return []any{nil}, nil
		},
		"GetLeaderGUID": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.LeaderGUID}, nil
			}
			return []any{nil}, nil
		},
		"GetMemberCount": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.MemberCount}, nil
			}
			return []any{nil}, nil
		},
		"GetMOTD": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.MOTD}, nil
			}
			return []any{nil}, nil
		},
		"GetInfo": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.Info}, nil
			}
			return []any{nil}, nil
		},
		// GetLeader mirrors FindPlayer(guild->GetLeaderGUID()): the leader's
		// Player object when online, nil otherwise (GuildMethods.h).
		"GetLeader": func(context.Context, []any) ([]any, error) {
			snap, ok := load()
			if !ok {
				return []any{nil}, nil
			}
			var out any
			if leader := playerObjectByGUID(live.Players, snap.LeaderGUID); leader != nil {
				out = leader
			}
			return []any{out}, nil
		},
		// GetMembers mirrors the C++ online-and-in-world filter
		// (GuildMethods.h: IsInWorld() && GetGuildId() == guild->GetId()).
		"GetMembers": func(ctx context.Context, _ []any) ([]any, error) {
			members := []any{}
			if live.Players == nil {
				return []any{members}, nil
			}
			for _, player := range live.Players() {
				if player == nil {
					continue
				}
				if inWorld, ok := player.Fields["InWorld"]; ok {
					if on, _ := inWorld.(bool); !on {
						continue
					}
				}
				if id, ok := objectGuildID(ctx, player); ok && id == guildID {
					members = append(members, player)
				}
			}
			return []any{members}, nil
		},
	}
	return &Object{Type: "Guild", Fields: map[string]any{"ID": uint32(guildID), "Name": snap.Name, "LeaderGUID": snap.LeaderGUID, "MemberCount": snap.MemberCount, "MOTD": snap.MOTD, "Info": snap.Info}, Methods: methods}
}

// ItemTemplateLive is the item_template row behind a Lua Item object: the
// fields Eluna's Item methods read through Item::GetTemplate(). The spell
// slots mirror MAX_ITEM_PROTO_SPELLS = 5 (ItemTemplate.h:597).
type ItemTemplateLive struct {
	Name                              string
	Class, SubClass, DisplayInfoID    uint32
	Quality, ItemLevel, RequiredLevel uint32
	BuyPrice                          int32
	SellPrice, InventoryType          uint32
	AllowableClass, AllowableRace     uint32
	Stackable                         int32
	ContainerSlots, StatsCount        uint32
	SpellIDs                          [5]uint32
	SpellTriggers                     [5]uint32
}

// ItemLive carries the template-data callback behind a Lua Item object.
// Load re-reads the item_template row (Eluna's methods read the live
// Item::GetTemplate()); a nil Load or a false return means the row is
// missing, and the template methods push nil — C++ would dereference a
// null template there, so Go takes the safe arm.
type ItemLive struct {
	LoadTemplate func() (ItemTemplateLive, bool)
}

// itemMaxStackCount mirrors ItemTemplate::GetMaxStackSize
// (ItemTemplate.h:686): Stackable <= 0 or the 2147483647 sentinel means
// effectively unlimited stacking.
func itemMaxStackCount(stackable int32) uint32 {
	if stackable == 2147483647 || stackable <= 0 {
		return 0x7FFFFFFE
	}
	return uint32(stackable)
}

// invTypeBag mirrors INVTYPE_BAG behind Item::IsBag (Item.h:102):
// an item is a bag exactly when its template inventory type is 18.
const invTypeBag = 18

// NewItemObject builds the Lua Item object surface for hooks that hand
// scripts a live item (Eluna's Push(Item*) via ElunaTemplate): the GUID,
// Entry and Count fields plus the template-read methods GetName,
// GetQuality, GetClass, GetSubClass, GetDisplayId, GetItemLevel,
// GetRequiredLevel, GetBuyPrice, GetSellPrice, GetInventoryType,
// GetAllowableClass, GetAllowableRace, GetMaxStackCount, GetBagSize,
// GetStatsCount, IsBag, GetSpellId and GetSpellTrigger (ItemMethods.h).
// The generic object methods supply GetEntry/GetGUID/GetGUIDLow/GetTypeId
// (object.go). The instance-bound surface (GetOwner, GetSlot, GetBagSlot,
// IsEquipped, IsBroken) and the mutating surface (SetOwner, SetBinding,
// SetCount, SetEnchantment, ClearEnchantment, SaveToDB) have no Go
// live-item model behind them and stay unbridged.
func NewItemObject(guid uint64, entry, count uint32, live ItemLive) *Object {
	load := live.LoadTemplate
	if load == nil {
		load = func() (ItemTemplateLive, bool) { return ItemTemplateLive{}, false }
	}
	templateValue := func(extract func(ItemTemplateLive) any) []any {
		tmpl, ok := load()
		if !ok {
			return []any{nil}
		}
		return []any{extract(tmpl)}
	}
	templateSpell := func(args []any, pick func(ItemTemplateLive, uint32) uint32) ([]any, error) {
		index, err := objectArgumentUint32(args, 0)
		if err != nil {
			return nil, err
		}
		// ItemMethods.h: index >= MAX_ITEM_PROTO_SPELLS is a Lua argument
		// error, raised before the template is touched.
		if index >= 5 {
			return nil, fmt.Errorf("valid SpellIndex expected")
		}
		tmpl, ok := load()
		if !ok {
			return []any{nil}, nil
		}
		return []any{pick(tmpl, index)}, nil
	}
	methods := map[string]ObjectMethod{
		"GetCount": func(context.Context, []any) ([]any, error) { return []any{count}, nil },
		"GetName": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.Name }), nil
		},
		"GetQuality": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.Quality }), nil
		},
		"GetClass": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.Class }), nil
		},
		"GetSubClass": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.SubClass }), nil
		},
		"GetDisplayId": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.DisplayInfoID }), nil
		},
		"GetItemLevel": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.ItemLevel }), nil
		},
		"GetRequiredLevel": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.RequiredLevel }), nil
		},
		"GetBuyPrice": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.BuyPrice }), nil
		},
		"GetSellPrice": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.SellPrice }), nil
		},
		"GetInventoryType": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.InventoryType }), nil
		},
		"GetAllowableClass": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.AllowableClass }), nil
		},
		"GetAllowableRace": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.AllowableRace }), nil
		},
		"GetMaxStackCount": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return itemMaxStackCount(t.Stackable) }), nil
		},
		// Bag::GetBagSize is the template ContainerSlots; a non-bag pushes 0.
		"GetBagSize": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any {
				if t.InventoryType != invTypeBag {
					return uint32(0)
				}
				return t.ContainerSlots
			}), nil
		},
		"GetStatsCount": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.StatsCount }), nil
		},
		"IsBag": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.InventoryType == invTypeBag }), nil
		},
		"GetSpellId": func(_ context.Context, args []any) ([]any, error) {
			return templateSpell(args, func(t ItemTemplateLive, index uint32) uint32 { return t.SpellIDs[index] })
		},
		"GetSpellTrigger": func(_ context.Context, args []any) ([]any, error) {
			return templateSpell(args, func(t ItemTemplateLive, index uint32) uint32 { return t.SpellTriggers[index] })
		},
	}
	return &Object{Type: "Item", Fields: map[string]any{"GUID": guid, "Entry": entry, "Count": count}, Methods: methods}
}

// MapSnapshot is the map DBC entry behind a Lua Map object: the fields
// Eluna's Map methods read through Map + sMapStore (MapMethods.h;
// MapEntry::IsDungeon/IsRaid/IsBattleArena/IsBattleground).
type MapSnapshot struct {
	Name                                       string
	IsDungeon, IsRaid, IsArena, IsBattleground bool
}

// MapLive carries the map-data callback behind a Lua Map object. Load
// re-reads the DBC entry; a nil Load or a false return means the entry is
// missing, and the DBC-backed methods push nil/false.
type MapLive struct {
	Load func() (MapSnapshot, bool)
}

// NewMapObject builds the Lua Map object surface for the MAP_EVENT_* and
// INSTANCE_EVENT_* hooks: GetMapId/GetInstanceId ride the generic object
// methods (object.go) and the DBC-backed GetName/IsDungeon/IsRaid/IsArena/
// IsBattleground come from the live MapLive (MapMethods.h). The remaining
// Map surface (GetHeight/GetDifficulty/GetPlayerCount/GetAreaId(x,y,z)/
// GetWorldObject/SetWeather/GetInstanceData/SaveInstanceData/GetPlayers)
// needs a live Map* with grid/difficulty state and stays unbridged.
func NewMapObject(mapID, instanceID uint32, live MapLive) *Object {
	load := live.Load
	if load == nil {
		load = func() (MapSnapshot, bool) { return MapSnapshot{}, false }
	}
	flag := func(extract func(MapSnapshot) bool) []any {
		snap, _ := load()
		return []any{extract(snap)}
	}
	methods := map[string]ObjectMethod{
		"GetName": func(context.Context, []any) ([]any, error) {
			snap, ok := load()
			if !ok {
				return []any{nil}, nil
			}
			return []any{snap.Name}, nil
		},
		"IsDungeon": func(context.Context, []any) ([]any, error) {
			return flag(func(s MapSnapshot) bool { return s.IsDungeon }), nil
		},
		"IsRaid": func(context.Context, []any) ([]any, error) {
			return flag(func(s MapSnapshot) bool { return s.IsRaid }), nil
		},
		"IsArena": func(context.Context, []any) ([]any, error) {
			return flag(func(s MapSnapshot) bool { return s.IsArena }), nil
		},
		"IsBattleground": func(context.Context, []any) ([]any, error) {
			return flag(func(s MapSnapshot) bool { return s.IsBattleground }), nil
		},
	}
	return &Object{Type: "Map", Fields: map[string]any{"MapId": mapID, "InstanceId": instanceID, "InWorld": true}, Methods: methods}
}

// GroupMemberLive is one live group roster entry behind a Lua Group object.
type GroupMemberLive struct {
	GUID      uint64
	Name      string
	SubGroup  uint8
	Assistant bool
}

// GroupSnapshot is the live group roster behind a Lua Group object.
type GroupSnapshot struct {
	LeaderGUID uint64
	Members    []GroupMemberLive
	IsRaid     bool
	IsBG       bool
}

// GroupLive carries the live-data callbacks behind a Lua Group object.
// Load re-reads the roster (Eluna methods read the live Group*); Players
// enumerates the online player objects for the GetMembers/GetLeader
// filters. Either may be nil.
type GroupLive struct {
	Load    func() (GroupSnapshot, bool)
	Players func() []*Object
}

// maxRaidSubGroups mirrors MAX_RAID_SUBGROUPS (Group.h:44,
// MAXRAIDSIZE/MAXGROUPSIZE = 8); groupFull sizes mirror MAXGROUPSIZE=5 and
// MAXRAIDSIZE=40 behind Group::IsFull (Group.cpp:2451).
const (
	maxRaidSubGroups = 8
	maxGroupSize     = 5
	maxRaidSize      = 40
)

// NewGroupObject builds the Lua Group object surface: the ID, GUID,
// LeaderGUID and member GUID fields plus the GetId/GetGUID/GetLeaderGUID/
// GetMembers/GetMembersCount/IsLeader/IsFull/IsRaidGroup/IsBGGroup/IsMember/
// IsAssistant/SameSubGroup/HasFreeSlotSubGroup/GetMemberGUID/GetMemberGroup
// methods (GroupMethods.h). GetMembers pushes the online Player objects for
// members with sessions, like the C++. It is the Go model of Eluna's
// Push(group) (LuaEngine/GroupHooks.cpp via ElunaTemplate), shared by the
// world engine's group event hooks. The mutating surface
// (AddMember/SetLeader/RemoveMember/Disband/ConvertToRaid/SetMembersGroup/
// SetTargetIcon/SendPacket, GroupMethods.h) is attached by the world
// engine's attachGroupMutations, which re-resolves the live group per call.
// groupGUID is the protocol group GUID (0x1F50 high
// bits, as in world group.go's groupGUID).
func NewGroupObject(groupGUID uint64, live GroupLive) *Object {
	load := live.Load
	if load == nil {
		load = func() (GroupSnapshot, bool) { return GroupSnapshot{}, false }
	}
	snap, _ := load()
	memberGUIDs := make([]uint64, 0, len(snap.Members))
	for _, m := range snap.Members {
		memberGUIDs = append(memberGUIDs, m.GUID)
	}
	memberByGUID := func(snap GroupSnapshot, guid uint64) *GroupMemberLive {
		for i := range snap.Members {
			if snap.Members[i].GUID == guid {
				return &snap.Members[i]
			}
		}
		return nil
	}
	methods := map[string]ObjectMethod{
		"GetId":   func(context.Context, []any) ([]any, error) { return []any{uint32(groupGUID & 0xFFFFFFFF)}, nil },
		"GetGUID": func(context.Context, []any) ([]any, error) { return []any{groupGUID}, nil },
		"GetLeaderGUID": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.LeaderGUID}, nil
			}
			return []any{nil}, nil
		},
		// GetMembers pushes the online Player objects for members with
		// sessions (GroupMethods.h), not the raw GUID list.
		"GetMembers": func(context.Context, []any) ([]any, error) {
			snap, _ := load()
			members := []any{}
			for _, m := range snap.Members {
				if player := playerObjectByGUID(live.Players, m.GUID); player != nil {
					members = append(members, player)
				}
			}
			return []any{members}, nil
		},
		"GetMembersCount": func(context.Context, []any) ([]any, error) {
			snap, _ := load()
			return []any{uint32(len(snap.Members))}, nil
		},
		"IsLeader": func(_ context.Context, args []any) ([]any, error) {
			guid, err := objectArgumentUint64(args, 0)
			if err != nil {
				return nil, err
			}
			snap, _ := load()
			return []any{guid == snap.LeaderGUID}, nil
		},
		// Group::IsFull (Group.cpp:2451): raid groups fill at MAXRAIDSIZE,
		// normal groups at MAXGROUPSIZE.
		"IsFull": func(context.Context, []any) ([]any, error) {
			snap, _ := load()
			limit := maxGroupSize
			if snap.IsRaid {
				limit = maxRaidSize
			}
			return []any{len(snap.Members) >= limit}, nil
		},
		"IsRaidGroup": func(context.Context, []any) ([]any, error) {
			snap, _ := load()
			return []any{snap.IsRaid}, nil
		},
		"IsBGGroup": func(context.Context, []any) ([]any, error) {
			snap, _ := load()
			return []any{snap.IsBG}, nil
		},
		"IsMember": func(_ context.Context, args []any) ([]any, error) {
			guid, err := objectArgumentUint64(args, 0)
			if err != nil {
				return nil, err
			}
			snap, _ := load()
			return []any{memberByGUID(snap, guid) != nil}, nil
		},
		"IsAssistant": func(_ context.Context, args []any) ([]any, error) {
			guid, err := objectArgumentUint64(args, 0)
			if err != nil {
				return nil, err
			}
			snap, _ := load()
			member := memberByGUID(snap, guid)
			return []any{member != nil && member.Assistant}, nil
		},
		// SameSubGroup compares the two players' subgroups; like the C++
		// (Group.cpp:1886) both must be group members.
		"SameSubGroup": func(_ context.Context, args []any) ([]any, error) {
			first, err := objectArgument(args)
			if err != nil {
				return nil, err
			}
			if len(args) < 2 {
				return nil, fmt.Errorf("argument 2 is required")
			}
			second, ok := args[1].(*Object)
			if !ok || second == nil {
				return nil, fmt.Errorf("object argument expected")
			}
			guid1, ok1 := objectUint64(first, "GUID")
			guid2, ok2 := objectUint64(second, "GUID")
			if !ok1 || !ok2 {
				return []any{false}, nil
			}
			snap, _ := load()
			m1 := memberByGUID(snap, guid1)
			m2 := memberByGUID(snap, guid2)
			if m1 == nil || m2 == nil {
				return []any{false}, nil
			}
			return []any{m1.SubGroup == m2.SubGroup}, nil
		},
		// HasFreeSlotSubGroup errors on subGroup >= MAX_RAID_SUBGROUPS like
		// the C++ luaL_argerror (GroupMethods.h); the counter array only
		// exists for raid groups (Group.cpp:2688), so non-raids are never
		// free.
		"HasFreeSlotSubGroup": func(_ context.Context, args []any) ([]any, error) {
			sub, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			if sub >= maxRaidSubGroups {
				return nil, fmt.Errorf("valid subGroup ID expected")
			}
			snap, _ := load()
			if !snap.IsRaid {
				return []any{false}, nil
			}
			count := 0
			for _, m := range snap.Members {
				if m.SubGroup == uint8(sub) {
					count++
				}
			}
			return []any{count < maxGroupSize}, nil
		},
		"GetMemberGUID": func(_ context.Context, args []any) ([]any, error) {
			if len(args) == 0 {
				return nil, fmt.Errorf("argument 1 is required")
			}
			name, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("argument 1 must be a string")
			}
			snap, _ := load()
			for _, m := range snap.Members {
				if m.Name == name {
					return []any{m.GUID}, nil
				}
			}
			return []any{uint64(0)}, nil
		},
		// GetMemberGroup returns MAX_RAID_SUBGROUPS+1 for non-members
		// (Group.cpp:2571).
		"GetMemberGroup": func(_ context.Context, args []any) ([]any, error) {
			guid, err := objectArgumentUint64(args, 0)
			if err != nil {
				return nil, err
			}
			snap, _ := load()
			if member := memberByGUID(snap, guid); member != nil {
				return []any{uint32(member.SubGroup)}, nil
			}
			return []any{uint32(maxRaidSubGroups + 1)}, nil
		},
	}
	return &Object{Type: "Group", Fields: map[string]any{"ID": uint32(groupGUID & 0xFFFFFFFF), "GUID": groupGUID, "LeaderGUID": snap.LeaderGUID, "Members": memberGUIDs}, Methods: methods}
}

func (r *Runtime) pushGuildLookup(state *lua.State, statement string, arg any) int {
	if r.config.CharacterDB == nil {
		state.PushNil()
		return 1
	}
	db := r.config.CharacterDB
	load := func() (uint32, GuildSnapshot, bool) {
		var snap GuildSnapshot
		var guildID, leaderGUID, memberCount int64
		if err := db.QueryRow(statement, arg).Scan(&guildID, &snap.Name, &leaderGUID, &snap.MOTD, &snap.Info); err != nil || guildID == 0 {
			return 0, GuildSnapshot{}, false
		}
		snap.LeaderGUID = uint64(leaderGUID)
		_ = db.QueryRow("SELECT COUNT(1) FROM guild_member WHERE guildid = ?", guildID).Scan(&memberCount)
		snap.MemberCount = uint32(memberCount)
		return uint32(guildID), snap, true
	}
	guildID, _, ok := load()
	if !ok {
		state.PushNil()
		return 1
	}
	PushObject(state, NewGuildObject(guildID, GuildLive{
		Load: func() (GuildSnapshot, bool) {
			_, snap, ok := load()
			return snap, ok
		},
		Players: r.config.PlayerProvider,
	}))
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
