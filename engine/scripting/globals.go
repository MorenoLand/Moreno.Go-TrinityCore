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
	var specialFlags int64
	if r.config.WorldDatabase != nil {
		_ = r.config.WorldDatabase.QueryRow("SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", uint32(questID)).Scan(&specialFlags)
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
	// IsRepeatable is Quest::IsRepeatable (QuestDef.h:284): SpecialFlags &
	// QUEST_SPECIAL_FLAGS_REPEATABLE (0x001) from quest_template_addon, not
	// the quest_template Flags daily/weekly bits; a missing addon row
	// means not repeatable, matching the session questIsRepeatable twin.
	methods["IsRepeatable"] = func(context.Context, []any) ([]any, error) { return []any{specialFlags&0x001 != 0}, nil }
	PushObject(state, &Object{Type: "Quest", Fields: map[string]any{"ID": uint32(id), "Name": title, "Title": title, "Level": uint32(level), "MinLevel": uint32(minLevel), "Flags": uint32(flags)}, Methods: methods})
	return 1
}

// itemLinkQualityColors mirrors ItemQualityColors (SharedDefines.h):
// grey, white, green, blue, purple, orange, light yellow, artifact.
var itemLinkQualityColors = []uint32{
	0xFF9D9D9D, 0xFFFFFFFF, 0xFF1EFF00, 0xFF0070DD,
	0xFFA335EE, 0xFFFF8000, 0xFFE6CC80, 0xFFE6CC80,
}

// getItemLink is GlobalMethods::GetItemLink (GlobalMethods.h:404-427,
// Trinity): "|c<quality-color>|Hitem:<entry>:0:0:0:0:0:0:0:0:0|h[<name>]|h|r"
// (the non-CLASSIC link — the commands_list.go list-mail twin format).
// An unknown entry is the C++ luaL_argerror; the locale argument is
// accepted and range-checked but Go names are enUS-only, so it has no
// effect.
func (r *Runtime) getItemLink(state *lua.State) int {
	entry := checkLuaUint64(state, 1)
	var locale uint64
	if state.Top() >= 2 && !state.IsNil(2) {
		locale = checkLuaUint64(state, 2)
	}
	if locale >= 7 { // TOTAL_LOCALES
		lua.ArgumentError(state, 2, "valid LocaleConstant expected")
		return 0
	}
	if r.config.WorldDatabase == nil || entry > math.MaxUint32 {
		lua.ArgumentError(state, 1, "valid ItemEntry expected")
		return 0
	}
	var name string
	var quality uint32
	if err := r.config.WorldDatabase.QueryRow("SELECT COALESCE(name, ''), COALESCE(Quality, 0) FROM item_template WHERE entry = ?", uint32(entry)).Scan(&name, &quality); err != nil {
		lua.ArgumentError(state, 1, "valid ItemEntry expected")
		return 0
	}
	color := uint32(0xFFFFFFFF)
	if int(quality) < len(itemLinkQualityColors) {
		color = itemLinkQualityColors[quality]
	}
	state.PushString(fmt.Sprintf("|c%08x|Hitem:%d:0:0:0:0:0:0:0:0:0|h[%s]|h|r", color, uint32(entry), name))
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
// SendPacket/SendPacketToRanked/Disband/AddMember/DeleteMember/SetMemberRank,
// GuildMethods.h) is attached by the world engine's attachGuildMutations,
// which re-resolves the live guild per call; snapshot-only objects (the
// globals) carry the read surface only.
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
	BuyCount                          uint32
	SellPrice, InventoryType          uint32
	AllowableClass, AllowableRace     uint32
	Stackable                         int32
	ContainerSlots, StatsCount        uint32
	Flags, BagFamily                  uint32
	StartQuest, ItemSet               uint32
	RandomProperty, RandomSuffix      int32
	MaxDurability                     uint32
	SpellIDs                          [5]uint32
	SpellTriggers                     [5]uint32
}

// ItemInstanceLive is the item_instance row plus the character_inventory
// position behind a Lua Item object: the fields Eluna's instance-bound
// Item methods read through Item itself (Item.h/Item.cpp). BagSlot is the
// container's slot, or INVENTORY_SLOT_BAG_0 (255) for a top-level item
// (Item.cpp:713-715, GetBagSlot); InBag is m_container != nullptr, i.e.
// the character_inventory bag column is non-zero.
type ItemInstanceLive struct {
	OwnerGUID         uint64
	Flags             uint32
	Count             uint32
	Durability        uint32
	Enchantments      string
	Slot              uint8
	BagSlot           uint8
	InBag             bool
	NotEmptyBag       bool
	BoundByEnchant    bool
	BOPTradeable      bool
	AllowedTradeGUIDs []uint64
	RandomPropertyId  int32
	OwnerLevel        uint32
}

// ItemLive carries the template-data callback behind a Lua Item object.
// Load re-reads the item_template row (Eluna's methods read the live
// Item::GetTemplate()); a nil Load or a false return means the row is
// missing, and the template methods push nil — C++ would dereference a
// null template there, so Go takes the safe arm. LoadInstance re-reads
// the item_instance row and inventory position the same way; a nil
// LoadInstance means the object is template-only (loot/quest items) and
// the instance methods push nil. Players enumerates the online player
// objects for GetOwner's FindPlayer-style lookup.
// ItemInstanceUpdate is one persisted item_instance column write behind
// a Lua Item mutation: only non-nil fields are written, so a caller that
// computed a new value from a fresh LoadInstance read cannot clobber
// unrelated columns. The world loader owns the SQL; the scripting layer
// owns the C++ argument order and flag/enchantment-string arithmetic.
type ItemInstanceUpdate struct {
	OwnerGUID    *uint64
	Flags        *uint32
	Count        *uint32
	Enchantments *string
	// EnchantSlot marks a SetEnchantment write (never ClearEnchantment):
	// the world layer sends Item::SetEnchantment's log pair for this slot.
	EnchantSlot *uint32
}

type ItemLive struct {
	LoadTemplate func() (ItemTemplateLive, bool)
	LoadInstance func() (ItemInstanceLive, bool)
	// UpdateInstance persists one ItemInstanceUpdate against the
	// item_instance row; nil on template-only objects, where mutations
	// have no row and are silent no-ops. Writes land immediately, so
	// SaveToDB is a no-op flush on top of them.
	UpdateInstance func(ItemInstanceUpdate) bool
	// EnchantmentExists is the SpellItemEnchantment DBC lookup behind
	// SetEnchantment's C++ store check; nil reads as not found.
	EnchantmentExists func(uint32) bool
	// InTrade is the goroutine-safe Item::IsInTrade read (Item.h:108):
	// the world's server-guarded mirror of mb_in_trade; nil reads as
	// not in trade.
	InTrade func(uint64) bool
	Players func() []*Object
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

// Template-class constants behind the ItemTemplate predicates
// (ItemTemplate.h:153/245/296/303/320/444/445).
const (
	itemClassConsumable           = 0
	itemClassTradeGoods           = 7
	itemSubclassPotion            = 1
	itemSubclassArmorEnchantment  = 14
	itemSubclassWeaponEnchantment = 15
	itemFlagConjured              = 0x00000002
	itemFlagBoundToAccount        = 0x08000000
	itemFlagSoulbound             = 0x00000001
	itemFlagUnlocked              = 0x00000004
	enchantmentCanSoulbound       = 0x00000001
	bagFamilyMaskCurrencyTokens   = 0x00002000
)

// itemMaxInspectedEnchantmentSlot mirrors MAX_INSPECTED_ENCHANTMENT_SLOT
// (ItemDefines.h:153); itemEquipmentSlotEnd mirrors EQUIPMENT_SLOT_END;
// inventorySlotBag0 mirrors INVENTORY_SLOT_BAG_0, the GetBagSlot value
// for a top-level item (Item.cpp:713-715).
const (
	itemMaxInspectedEnchantmentSlot = 7
	itemEquipmentSlotEnd            = 19
	inventorySlotBag0               = 255
)

// item bag-slot ranges behind Player::IsBagPos (Player.cpp:10174-10182):
// top-level bag slots 19-22 and bank bag slots 67-73, both with the
// INVENTORY_SLOT_BAG_0 (255) bag.
const (
	itemInventoryBagSlotStart = 19
	itemInventoryBagSlotEnd   = 23
	itemBankBagSlotStart      = 67
	itemBankBagSlotEnd        = 74
)

// itemEnchantmentID reads one enchantment ID out of the item_instance
// enchantments string: space-separated (id, duration, charges) triples
// per slot (Item.cpp:354-356 save, LoadFromDB field 6), so slot s lives
// at fields[s*3]; a short string reads 0, like an empty slot.
func itemEnchantmentID(raw string, slot uint32) uint32 {
	fields := strings.Fields(raw)
	index := int(slot) * 3
	if index >= len(fields) {
		return 0
	}
	value, err := strconv.ParseUint(fields[index], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(value)
}

// itemSetEnchantmentString rewrites one enchantment triple (id, duration,
// charges) in an item_instance enchantments string, padding short strings
// with zero triples first. Setting id 0 is the ClearEnchantment shape:
// Item::ClearEnchantment zeroes the whole triple (Item.cpp:876-884), and
// Item::SetEnchantment writes duration/charges 0 through Eluna
// (ItemMethods.h:716), so both arms share this helper.
func itemSetEnchantmentString(raw string, slot, id uint32) string {
	fields := strings.Fields(raw)
	need := int(slot)*3 + 3
	for len(fields) < need {
		fields = append(fields, "0")
	}
	fields[int(slot)*3] = strconv.FormatUint(uint64(id), 10)
	fields[int(slot)*3+1] = "0"
	fields[int(slot)*3+2] = "0"
	return strings.Join(fields, " ")
}

// NewItemObject builds the Lua Item object surface for hooks that hand
// scripts a live item (Eluna's Push(Item*) via ElunaTemplate): the GUID,
// Entry and Count fields plus the template-read methods GetName,
// GetQuality, GetClass, GetSubClass, GetDisplayId, GetItemLevel,
// GetRequiredLevel, GetBuyPrice, GetBuyCount, GetSellPrice,
// GetInventoryType, GetAllowableClass, GetAllowableRace, GetMaxStackCount,
// GetBagSize, GetStatsCount, GetItemSet, GetRandomProperty,
// GetRandomSuffix, HasQuest, IsBag, IsPotion, IsBoundAccountWide,
// IsWeaponVellum, IsArmorVellum, IsConjuredConsumable, IsCurrencyToken,
// GetSpellId and GetSpellTrigger (ItemMethods.h), plus the instance-bound
// reads GetOwner, GetOwnerGUID, GetSlot, GetBagSlot, GetEnchantmentId,
// IsSoulBound, IsEquipped, IsBroken, IsLocked, IsBoundByEnchant,
// IsNotEmptyBag, IsInBag, IsNotBoundToPlayer, CanBeTraded and GetItemLink
// when the object carries a LoadInstance.
// The generic object methods supply GetEntry/GetGUID/GetGUIDLow/GetTypeId
// (object.go). The mutating surface (SetOwner, SetBinding, SetCount,
// SetEnchantment, ClearEnchantment, SaveToDB) writes through the world's
// UpdateInstance callback straight to the item_instance row, so a later
// LoadInstance read sees the write; the world layer also sends the owner
// session the values block for the changed fields (plus the
// SetEnchantment log pair), so a live client sees the change at once.
// IsInTrade reads the world's
// server-guarded mb_in_trade mirror (see trade.go), never the
// session trade state, so it is safe from any Lua goroutine.
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
	// instanceValue is the LoadInstance twin of templateValue: a
	// template-only object (loot/quest items, no item_instance row) reads
	// nil from every instance method, like a missing template row.
	instanceValue := func(extract func(ItemInstanceLive) any) []any {
		if live.LoadInstance == nil {
			return []any{nil}
		}
		inst, ok := live.LoadInstance()
		if !ok {
			return []any{nil}
		}
		return []any{extract(inst)}
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
		// Item::SetCount writes ITEM_FIELD_STACK_COUNT (Item.cpp:1202),
		// so GetCount re-reads the instance row when one exists and
		// the construction-time count stays the template-only arm.
		"GetCount": func(context.Context, []any) ([]any, error) {
			if live.LoadInstance != nil {
				if inst, ok := live.LoadInstance(); ok {
					return []any{inst.Count}, nil
				}
			}
			return []any{count}, nil
		},
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
		// ItemTemplate::GetBuyCount (ItemMethods.h:510): pure template read.
		"GetBuyCount": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.BuyCount }), nil
		},
		// ItemTemplate::GetItemSet (ItemMethods.h:625): pure template read.
		"GetItemSet": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.ItemSet }), nil
		},
		// ItemTemplate::RandomProperty (ItemMethods.h:606): pure template read.
		"GetRandomProperty": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.RandomProperty }), nil
		},
		// ItemTemplate::RandomSuffix (ItemMethods.h:613): pure template read.
		"GetRandomSuffix": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any { return t.RandomSuffix }), nil
		},
		// Item::hasQuest (Item.h:175, ItemMethods.h:176): StartQuest == quest.
		"HasQuest": func(_ context.Context, args []any) ([]any, error) {
			quest, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			return templateValue(func(t ItemTemplateLive) any { return t.StartQuest == quest }), nil
		},
		// ItemTemplate::IsPotion (ItemTemplate.h:700): Class == 0 (CONSUMABLE)
		// && SubClass == 1 (POTION).
		"IsPotion": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any {
				return t.Class == itemClassConsumable && t.SubClass == itemSubclassPotion
			}), nil
		},
		// ItemTemplate::IsWeaponVellum (ItemTemplate.h:701): Class == 7
		// (TRADE_GOODS) && SubClass == 15 (WEAPON_ENCHANTMENT).
		"IsWeaponVellum": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any {
				return t.Class == itemClassTradeGoods && t.SubClass == itemSubclassWeaponEnchantment
			}), nil
		},
		// ItemTemplate::IsArmorVellum (ItemTemplate.h:702): Class == 7
		// (TRADE_GOODS) && SubClass == 14 (ARMOR_ENCHANTMENT).
		"IsArmorVellum": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any {
				return t.Class == itemClassTradeGoods && t.SubClass == itemSubclassArmorEnchantment
			}), nil
		},
		// ItemTemplate::IsConjuredConsumable (ItemTemplate.h:703):
		// Class == 0 (CONSUMABLE) && Flags & ITEM_FLAG_CONJURED (0x2).
		"IsConjuredConsumable": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any {
				return t.Class == itemClassConsumable && t.Flags&itemFlagConjured != 0
			}), nil
		},
		// ItemTemplate::IsCurrencyToken (ItemTemplate.h:684):
		// BagFamily & BAG_FAMILY_MASK_CURRENCY_TOKENS (0x2000).
		"IsCurrencyToken": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any {
				return t.BagFamily&bagFamilyMaskCurrencyTokens != 0
			}), nil
		},
		// Item::IsBoundAccountWide (ItemMethods.h:32 via ItemTemplate.h:179):
		// Flags & ITEM_FLAG_IS_BOUND_TO_ACCOUNT (0x08000000) — pure
		// template read despite the instance-sounding name.
		"IsBoundAccountWide": func(context.Context, []any) ([]any, error) {
			return templateValue(func(t ItemTemplateLive) any {
				return t.Flags&itemFlagBoundToAccount != 0
			}), nil
		},
		// Item::GetOwnerGUID (ItemMethods.h:336): the item_instance
		// owner_guid column.
		"GetOwnerGUID": func(context.Context, []any) ([]any, error) {
			return instanceValue(func(inst ItemInstanceLive) any { return inst.OwnerGUID }), nil
		},
		// Item::GetOwner (ItemMethods.h:351): FindPlayer(ownerGUID),
		// nil when the owner is offline — same shape as Guild GetLeader.
		"GetOwner": func(context.Context, []any) ([]any, error) {
			if live.LoadInstance == nil {
				return []any{nil}, nil
			}
			inst, ok := live.LoadInstance()
			if !ok {
				return []any{nil}, nil
			}
			var out any
			if owner := playerObjectByGUID(live.Players, inst.OwnerGUID); owner != nil {
				out = owner
			}
			return []any{out}, nil
		},
		// Item::GetSlot / GetBagSlot (ItemMethods.h:384/395): the
		// character_inventory slot and the container's slot (255 for a
		// top-level item, Item.cpp:713-715).
		"GetSlot": func(context.Context, []any) ([]any, error) {
			return instanceValue(func(inst ItemInstanceLive) any { return inst.Slot }), nil
		},
		"GetBagSlot": func(context.Context, []any) ([]any, error) {
			return instanceValue(func(inst ItemInstanceLive) any { return inst.BagSlot }), nil
		},
		// Item::GetEnchantmentId (ItemMethods.h:407): the argerror on
		// slot >= MAX_INSPECTED_ENCHANTMENT_SLOT is raised before the
		// instance is touched, like GetSpellId's index arm.
		"GetEnchantmentId": func(_ context.Context, args []any) ([]any, error) {
			slot, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			if slot >= itemMaxInspectedEnchantmentSlot {
				return nil, fmt.Errorf("valid EnchantmentSlot expected")
			}
			return instanceValue(func(inst ItemInstanceLive) any {
				return itemEnchantmentID(inst.Enchantments, slot)
			}), nil
		},
		// Item::IsSoulBound (Item.h:81): ITEM_FIELD_FLAGS &
		// ITEM_FIELD_FLAG_SOULBOUND; the item_instance flags column IS
		// that word (Item.cpp:349 save, :460 load).
		"IsSoulBound": func(context.Context, []any) ([]any, error) {
			return instanceValue(func(inst ItemInstanceLive) any {
				return inst.Flags&itemFlagSoulbound != 0
			}), nil
		},
		// Item::IsLocked (Item.h:101): NOT HasFlag(UNLOCKED).
		"IsLocked": func(context.Context, []any) ([]any, error) {
			return instanceValue(func(inst ItemInstanceLive) any {
				return inst.Flags&itemFlagUnlocked == 0
			}), nil
		},
		// Item::IsBroken (Item.h:105): template MaxDurability > 0 and
		// instance durability == 0.
		"IsBroken": func(context.Context, []any) ([]any, error) {
			tmpl, ok := load()
			if !ok {
				return []any{nil}, nil
			}
			return instanceValue(func(inst ItemInstanceLive) any {
				return tmpl.MaxDurability > 0 && inst.Durability == 0
			}), nil
		},
		// Item::IsInBag (Item.h:131): m_container != nullptr.
		"IsInBag": func(context.Context, []any) ([]any, error) {
			return instanceValue(func(inst ItemInstanceLive) any { return inst.InBag }), nil
		},
		// Item::IsEquipped (Item.cpp:715-718): not in a bag and the
		// slot is below EQUIPMENT_SLOT_END.
		"IsEquipped": func(context.Context, []any) ([]any, error) {
			return instanceValue(func(inst ItemInstanceLive) any {
				return !inst.InBag && inst.Slot < itemEquipmentSlotEnd
			}), nil
		},
		// Item::IsInTrade (Item.h:107-108, 215): mb_in_trade is set
		// only by setAcceptTradeMode and cleared by
		// clearAcceptTradeMode (TradeHandler.cpp:209-248), so the
		// flag lives only inside the accept-process window; the
		// world mirror behind live.InTrade tracks exactly that
		// window. A template-only object reads false: a fresh Item
		// constructs with mb_in_trade false, and loot items never
		// enter a trade window.
		"IsInTrade": func(context.Context, []any) ([]any, error) {
			if live.InTrade == nil {
				return []any{false}, nil
			}
			return []any{live.InTrade(guid)}, nil
		},
		// Item::IsNotEmptyBag (Item.cpp:298-303): ToBag() non-nil and
		// non-empty; a non-bag reads false.
		"IsNotEmptyBag": func(context.Context, []any) ([]any, error) {
			return instanceValue(func(inst ItemInstanceLive) any { return inst.NotEmptyBag }), nil
		},
		// Item::IsBoundByEnchant (Item.cpp:771-785): any of the 12
		// enchantment slots whose SpellItemEnchantment entry carries
		// ENCHANTMENT_CAN_SOULBOUND (DBCEnums.h:394, 0x01) — computed
		// by the world loader, which owns the DBC store.
		"IsBoundByEnchant": func(context.Context, []any) ([]any, error) {
			return instanceValue(func(inst ItemInstanceLive) any { return inst.BoundByEnchant }), nil
		},
		// Item::IsBindedNotWith (Item.cpp:1039-1055): not soulbound
		// reads false; the owner reads false; a BOP-tradeable item
		// reads false for a player on its allowed list
		// (item_soulbound_trade_data, the bindLootSoulboundTradeable
		// twin); a bound-account-wide template reads false for
		// anyone. The player argument is CHECKOBJ<Player>: a
		// non-object argument errors, like objectArgument.
		"IsNotBoundToPlayer": func(_ context.Context, args []any) ([]any, error) {
			player, err := objectArgument(args)
			if err != nil {
				return nil, err
			}
			playerGUID, ok := objectUint64(player, "GUID")
			if !ok {
				return nil, fmt.Errorf("object argument expected")
			}
			if live.LoadInstance == nil {
				return []any{nil}, nil
			}
			inst, ok := live.LoadInstance()
			if !ok {
				return []any{nil}, nil
			}
			if inst.Flags&itemFlagSoulbound == 0 {
				return []any{false}, nil
			}
			if inst.OwnerGUID == playerGUID {
				return []any{false}, nil
			}
			if inst.BOPTradeable {
				for _, allowed := range inst.AllowedTradeGUIDs {
					if allowed == playerGUID {
						return []any{false}, nil
					}
				}
			}
			if tmpl, ok := load(); ok && tmpl.Flags&itemFlagBoundToAccount != 0 {
				return []any{false}, nil
			}
			return []any{true}, nil
		},
		// Item::CanBeTraded (Item.cpp:720-743, Eluna mail-only arm:
		// trade defaults false, so a soulbound item passes only via
		// the mail + bound-account-wide arm). Template-only objects
		// (loot-window, guild-bank) read nil, like every other
		// instance method — their loot-generated state is unmodeled,
		// and the owner legs (CanUnequipItem, the owner's loot GUID)
		// have no session behind the object, so they are skipped.
		"CanBeTraded": func(_ context.Context, args []any) ([]any, error) {
			mail := false
			if len(args) > 0 && args[0] != nil {
				if b, ok := args[0].(bool); ok {
					mail = b
				} else if number, ok := numericValue(args[0]); ok {
					mail = number != 0
				} else {
					return nil, fmt.Errorf("argument 1 must be boolean")
				}
			}
			if live.LoadInstance == nil {
				return []any{nil}, nil
			}
			inst, ok := live.LoadInstance()
			if !ok {
				return []any{nil}, nil
			}
			tmpl, tmplOK := load()
			if inst.Flags&itemFlagSoulbound != 0 && !(mail && tmplOK && tmpl.Flags&itemFlagBoundToAccount != 0) {
				return []any{false}, nil
			}
			if tmplOK && tmpl.InventoryType == invTypeBag {
				bagPos := !inst.InBag && ((inst.Slot >= itemInventoryBagSlotStart && inst.Slot < itemInventoryBagSlotEnd) ||
					(inst.Slot >= itemBankBagSlotStart && inst.Slot < itemBankBagSlotEnd))
				if bagPos || inst.NotEmptyBag {
					return []any{false}, nil
				}
			}
			if inst.BoundByEnchant {
				return []any{false}, nil
			}
			return []any{true}, nil
		},
		// ItemMethods.h GetItemLink (Trinity): the quality color plus
		// "|Hitem:<entry>:<perm>:<sock>:<sock2>:<sock3>:<bonus>:
		// <randomProperty>:<suffixFactor>:<ownerLevel>|h[<name>]|h|r".
		// The locale argument is range-checked (TOTAL_LOCALES 7) but
		// Go names are enUS-only, like the global GetItemLink twin;
		// the suffix factor has no Go model (C++ PROPERTY_SEED is
		// runtime-only, set by SetItemRandomProperties at creation),
		// and the random-property name suffix is not appended (no
		// ItemRandomSuffix/ItemRandomProperties DBC read in Go), so
		// both print 0 / the bare template name.
		"GetItemLink": func(_ context.Context, args []any) ([]any, error) {
			var locale uint32
			if len(args) > 0 && args[0] != nil {
				parsed, err := objectArgumentUint32(args, 0)
				if err != nil {
					return nil, err
				}
				locale = parsed
			}
			if locale >= 7 {
				return nil, fmt.Errorf("valid LocaleConstant expected")
			}
			tmpl, ok := load()
			if !ok {
				return []any{nil}, nil
			}
			if live.LoadInstance == nil {
				return []any{nil}, nil
			}
			inst, ok := live.LoadInstance()
			if !ok {
				return []any{nil}, nil
			}
			color := uint32(0xFFFFFFFF)
			if int(tmpl.Quality) < len(itemLinkQualityColors) {
				color = itemLinkQualityColors[tmpl.Quality]
			}
			link := fmt.Sprintf("|c%08x|Hitem:%d:%d:%d:%d:%d:%d:%d:%d:%d|h[%s]|h|r",
				color, entry,
				itemEnchantmentID(inst.Enchantments, 0),
				itemEnchantmentID(inst.Enchantments, 2),
				itemEnchantmentID(inst.Enchantments, 3),
				itemEnchantmentID(inst.Enchantments, 4),
				itemEnchantmentID(inst.Enchantments, 5),
				inst.RandomPropertyId, 0, inst.OwnerLevel, tmpl.Name)
			return []any{link}, nil
		},
		// Item::SetOwnerGUID via Eluna SetOwner (ItemMethods.h:650):
		// the player object's GUID column becomes the row's owner.
		// A template-only object has no row, so the write is a
		// silent no-op, like the template-only nil reads.
		"SetOwner": func(_ context.Context, args []any) ([]any, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("argument 1 is required")
			}
			player, ok := args[0].(*Object)
			if !ok || player == nil || player.Type != "Player" {
				return nil, fmt.Errorf("argument 1 must be a player object")
			}
			playerGUID, ok := objectUint64(player, "GUID")
			if !ok {
				return nil, fmt.Errorf("argument 1 must be a player object")
			}
			if live.UpdateInstance != nil {
				live.UpdateInstance(ItemInstanceUpdate{OwnerGUID: &playerGUID})
			}
			return []any{}, nil
		},
		// Item::SetBinding (Item.h:80): ApplyModFlag on the soulbound
		// bit of ITEM_FIELD_FLAGS, plus SetState(ITEM_CHANGED) — the
		// state flag is the persistence mark, and Go writes land
		// immediately, so there is no separate dirty bit to set.
		"SetBinding": func(_ context.Context, args []any) ([]any, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("argument 1 is required")
			}
			soulbound, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("argument 1 must be boolean")
			}
			if live.LoadInstance != nil && live.UpdateInstance != nil {
				if inst, ok := live.LoadInstance(); ok {
					flags := inst.Flags
					if soulbound {
						flags |= itemFlagSoulbound
					} else {
						flags &^= itemFlagSoulbound
					}
					live.UpdateInstance(ItemInstanceUpdate{Flags: &flags})
				}
			}
			return []any{}, nil
		},
		// Item::SetCount (Item.cpp:1202): the stack-count column;
		// the C++ trade-data refresh leg has no Go trade model.
		"SetCount": func(_ context.Context, args []any) ([]any, error) {
			value, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			if live.UpdateInstance != nil {
				live.UpdateInstance(ItemInstanceUpdate{Count: &value})
			}
			return []any{}, nil
		},
		// Eluna SetEnchantment (ItemMethods.h:695): owner first (no
		// online owner reads false), then the enchant DBC check
		// (unknown id reads false), then the slot argerror — the
		// C++ order, including its argerror naming argument 2
		// even though the slot is the third Lua argument, which Go
		// reports as the slot message itself. ApplyEnchantment's
		// stat legs have no Go model, so only the id triple lands;
		// the world layer sends the values block plus the
		// SetEnchantment log pair for the slot.
		"SetEnchantment": func(_ context.Context, args []any) ([]any, error) {
			if live.LoadInstance == nil || live.UpdateInstance == nil {
				return []any{false}, nil
			}
			inst, ok := live.LoadInstance()
			if !ok {
				return []any{false}, nil
			}
			if playerObjectByGUID(live.Players, inst.OwnerGUID) == nil {
				return []any{false}, nil
			}
			enchant, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			if live.EnchantmentExists == nil || !live.EnchantmentExists(enchant) {
				return []any{false}, nil
			}
			slot, err := objectArgumentUint32(args, 1)
			if err != nil {
				return nil, err
			}
			if slot >= itemMaxInspectedEnchantmentSlot {
				return nil, fmt.Errorf("valid EnchantmentSlot expected")
			}
			enchants := itemSetEnchantmentString(inst.Enchantments, slot, enchant)
			live.UpdateInstance(ItemInstanceUpdate{Enchantments: &enchants, EnchantSlot: &slot})
			return []any{true}, nil
		},
		// Eluna ClearEnchantment (ItemMethods.h:729): same owner and
		// slot gates as SetEnchantment; an empty slot reads false
		// instead of writing.
		"ClearEnchantment": func(_ context.Context, args []any) ([]any, error) {
			if live.LoadInstance == nil || live.UpdateInstance == nil {
				return []any{false}, nil
			}
			inst, ok := live.LoadInstance()
			if !ok {
				return []any{false}, nil
			}
			if playerObjectByGUID(live.Players, inst.OwnerGUID) == nil {
				return []any{false}, nil
			}
			slot, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			if slot >= itemMaxInspectedEnchantmentSlot {
				return nil, fmt.Errorf("valid EnchantmentSlot expected")
			}
			if itemEnchantmentID(inst.Enchantments, slot) == 0 {
				return []any{false}, nil
			}
			enchants := itemSetEnchantmentString(inst.Enchantments, slot, 0)
			live.UpdateInstance(ItemInstanceUpdate{Enchantments: &enchants})
			return []any{true}, nil
		},
		// Eluna SaveToDB (ItemMethods.h:757): Item::SaveToDB writes
		// the item_instance row, but every Go mutation above writes
		// that row immediately, so there is nothing left to flush.
		"SaveToDB": func(context.Context, []any) ([]any, error) {
			return []any{}, nil
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

// MapPlayerLive is one live player behind a Lua Map object: the player
// object plus the fields MapMethods.h's GetPlayers/GetPlayerCount filters
// read (a session behind the player; GetPlayersCountExceptGMs skips GMs).
type MapPlayerLive struct {
	Player *Object
	IsGM   bool
	Team   uint32
}

// MapLive carries the live-data callbacks behind a Lua Map object. Load
// re-reads the DBC entry; a nil Load or a false return means the entry is
// missing, and the DBC-backed methods push nil/false. Difficulty re-reads
// the per-instance spawn mode (Map::GetDifficulty is GetSpawnMode); a nil
// callback or a false return means normal mode. Players enumerates the
// worldReady players on this map instance; a nil callback means none.
// SetWeather applies a zone weather change; a nil callback drops it.
// WorldObject resolves a GUID to the Lua object Eluna pushes for
// MapMethods.h GetWorldObject (Map.cpp GetPlayer/GetCreature/GetPet/
// GetGameObject/GetCorpse); a nil callback means no object resolves.
type MapLive struct {
	Load         func() (MapSnapshot, bool)
	InstanceData func() *Object
	Difficulty   func(context.Context) (int64, bool)
	Players      func() []MapPlayerLive
	SetWeather   func(context.Context, uint32, uint32, float32)
	// SetZoneWeatherOverride is the Go-only Map::SetZoneWeather twin
	// (Map.cpp:4783): C++ Eluna exposes no zone-override method (its
	// Map SetWeather is the Weather::SetWeather type/grade arm), so
	// boss-script ports that need raw states (LK fog/blacksnow) call
	// the Map object's SetZoneWeather (zone, state, grade) instead.
	SetZoneWeatherOverride func(context.Context, uint32, uint32, float32)
	WorldObject            func(context.Context, uint64) *Object
	AreaID                 func(x, y, z float32) uint32
	// Height reads the .map grid height for Map::GetHeight's Eluna
	// call shape; false means no readable height chunk, and the
	// method turns that (and the INVALID_HEIGHT sentinel) into nil.
	Height func(x, y float32) (float32, bool)
}

// NewMapObject builds the Lua Map object surface for the MAP_EVENT_* and
// INSTANCE_EVENT_* hooks: GetMapId/GetInstanceId ride the generic object
// methods (object.go) and the DBC-backed GetName/IsDungeon/IsRaid/IsArena/
// IsBattleground come from the live MapLive (MapMethods.h). The live legs
// (IsEmpty/IsHeroic/GetDifficulty/GetPlayerCount/GetPlayers/SetWeather/
// SetZoneWeather/GetInstanceData/SaveInstanceData/GetAreaId/GetHeight) read the MapLive
// callbacks.
// GetWorldObject dispatches on
// the GUID high part through the live WorldObject callback (nil callback
// or nil return reads nil, matching Eluna's Push(nullptr) arm).
func NewMapObject(mapID, instanceID uint32, live MapLive) *Object {
	load := live.Load
	if load == nil {
		load = func() (MapSnapshot, bool) { return MapSnapshot{}, false }
	}
	flag := func(extract func(MapSnapshot) bool) []any {
		snap, _ := load()
		return []any{extract(snap)}
	}
	// difficulty is the per-instance spawn mode (Map.h:450 GetDifficulty
	// is GetSpawnMode); non-instance maps and missing rows read normal.
	difficulty := func(ctx context.Context) int64 {
		if live.Difficulty == nil {
			return 0
		}
		diff, ok := live.Difficulty(ctx)
		if !ok {
			return 0
		}
		return diff
	}
	players := func() []MapPlayerLive {
		if live.Players == nil {
			return nil
		}
		return live.Players()
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
		// IsEmpty is the MapMethods.h doc rule (no Players on the map);
		// this fork's Map.h exposes only the negation, HavePlayers
		// (Map.h:474, !m_mapRefManager.isEmpty()).
		"IsEmpty": func(context.Context, []any) ([]any, error) {
			return []any{len(players()) == 0}, nil
		},
		// IsHeroic is Map::IsHeroic (Map.cpp:4249-4252): raids heroic
		// at difficulty >= 2 (10-man heroic), dungeons at >= 1.
		"IsHeroic": func(ctx context.Context, _ []any) ([]any, error) {
			snap, _ := load()
			diff := difficulty(ctx)
			if snap.IsRaid {
				return []any{diff >= 2}, nil
			}
			return []any{diff >= 1}, nil
		},
		"GetDifficulty": func(ctx context.Context, _ []any) ([]any, error) {
			return []any{difficulty(ctx)}, nil
		},
		// GetPlayerCount is Map::GetPlayersCountExceptGMs
		// (Map.cpp:3649): GMs skipped, npcbots have no Go twin.
		"GetPlayerCount": func(context.Context, []any) ([]any, error) {
			count := uint32(0)
			for _, p := range players() {
				if !p.IsGM {
					count++
				}
			}
			return []any{count}, nil
		},
		// GetPlayers is MapMethods.h: team defaults to TEAM_NEUTRAL
		// (2, all teams); a session behind the player is the C++
		// GetSession gate, already applied by the live callback.
		"GetPlayers": func(_ context.Context, args []any) ([]any, error) {
			team := uint32(2)
			if len(args) > 0 {
				var err error
				team, err = objectArgumentUint32(args, 0)
				if err != nil {
					return nil, err
				}
			}
			list := []any{}
			for _, p := range players() {
				if p.Player == nil {
					continue
				}
				if team < 2 && p.Team != team {
					continue
				}
				list = append(list, p.Player)
			}
			return []any{list}, nil
		},
		// SetWeather is MapMethods.h (zone, type, grade, all required).
		// The C++ Trinity arm no-ops when the zone has no weather
		// record (GetOrGenerateZoneDefaultWeather returns nil); the
		// Go setZoneWeather twin no-ops the same way.
		"SetWeather": func(ctx context.Context, args []any) ([]any, error) {
			zone, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			wtype, err := objectArgumentUint32(args, 1)
			if err != nil {
				return nil, err
			}
			if len(args) < 3 {
				return nil, fmt.Errorf("argument 3 is required")
			}
			grade, ok := numericValue(args[2])
			if !ok {
				return nil, fmt.Errorf("argument 3 must be a number")
			}
			if live.SetWeather != nil {
				live.SetWeather(ctx, zone, wtype, float32(grade))
			}
			return nil, nil
		},
		// SetZoneWeather is the Go-only Map::SetZoneWeather twin
		// (Map.cpp:4783): (zone, state, grade, all required). C++
		// Eluna has no such method, so this exists only for
		// boss-script ports that need raw states FOG/DRIZZLE/
		// BLACKSNOW, which the type/grade SetWeather arm can never
		// derive. A nil callback drops it, like SetWeather.
		"SetZoneWeather": func(ctx context.Context, args []any) ([]any, error) {
			zone, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			state, err := objectArgumentUint32(args, 1)
			if err != nil {
				return nil, err
			}
			if len(args) < 3 {
				return nil, fmt.Errorf("argument 3 is required")
			}
			grade, ok := numericValue(args[2])
			if !ok {
				return nil, fmt.Errorf("argument 3 must be a number")
			}
			if live.SetZoneWeatherOverride != nil {
				live.SetZoneWeatherOverride(ctx, zone, state, float32(grade))
			}
			return nil, nil
		},
		// GetWorldObject is MapMethods.h (guid): the GUID-high-part
		// dispatch lives in the world-side WorldObject callback so
		// this package keeps its data-source decoupling; an
		// unknown high part or an unresolvable GUID reads nil,
		// matching Eluna's default arm (no Push value).
		"GetWorldObject": func(ctx context.Context, args []any) ([]any, error) {
			guid, err := objectArgumentUint64(args, 0)
			if err != nil {
				return nil, err
			}
			if live.WorldObject == nil {
				return []any{nil}, nil
			}
			return []any{live.WorldObject(ctx, guid)}, nil
		},
		// GetAreaId is MapMethods.h (x, y, z, phasemask = 1):
		// Map::GetAreaId (Map.cpp:2595) — the WMO area when the point
		// sits on WMO ground, else the grid area, else the map
		// entry's AreaTableID. The world-side AreaID callback runs
		// that same ladder (Server.zoneAndAreaID); a nil callback
		// reads 0. The phasemask argument is accepted and ignored:
		// Go's WMO/grid area model has no phase dimension, and C++
		// GetAreaId's WMO-vs-grid-height comparison has no Go twin,
		// so the WMO leg uses the z >= ground - 2 tolerance only.
		"GetAreaId": func(_ context.Context, args []any) ([]any, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("argument %d is required", len(args)+1)
			}
			coords := make([]float32, 3)
			for i := 0; i < 3; i++ {
				value, ok := numericValue(args[i])
				if !ok {
					return nil, fmt.Errorf("argument %d must be a number", i+1)
				}
				coords[i] = float32(value)
			}
			if live.AreaID == nil {
				return []any{uint32(0)}, nil
			}
			return []any{live.AreaID(coords[0], coords[1], coords[2])}, nil
		},
		// GetHeight is MapMethods.h (x, y, phasemask = 1):
		// Map::GetHeight for the Eluna call shape (Map.cpp:2489 +
		// Map.h:569), where z is MAX_HEIGHT (100000). The static-vmap
		// ray from that height down 50 (StaticMapTree::getHeight)
		// and the dynamic-floor ray from the same height
		// (DynamicMapTree::getHeight) can only hit geometry within
		// 50 of 100000, which no terrain or gameobject floor reaches,
		// so the Eluna shape reduces exactly to the .map grid height
		// the world-side Height callback reads. The phasemask
		// argument is accepted and validated but ignored: the
		// dynamic leg it feeds drops out for the reason above. An
		// invalid grid height reads nil, matching Eluna's
		// z != INVALID_HEIGHT push gate (Map.h:285).
		"GetHeight": func(_ context.Context, args []any) ([]any, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("argument %d is required", len(args)+1)
			}
			coords := make([]float32, 2)
			for i := 0; i < 2; i++ {
				value, ok := numericValue(args[i])
				if !ok {
					return nil, fmt.Errorf("argument %d must be a number", i+1)
				}
				coords[i] = float32(value)
			}
			if len(args) > 2 {
				if _, ok := numericValue(args[2]); !ok {
					return nil, fmt.Errorf("argument 3 must be a number")
				}
			}
			if live.Height == nil {
				return []any{nil}, nil
			}
			height, ok := live.Height(coords[0], coords[1])
			if !ok || height == -100000 {
				return []any{nil}, nil
			}
			return []any{height}, nil
		},
		// GetInstanceData returns the instance's Lua data table when
		// the world supplies one (MapMethods.h: the ElunaInstanceAI
		// table, nil when the cast fails); the world gates it on
		// instance-script bindings, and the returned object is the
		// same handle the INSTANCE_EVENT_* hooks receive.
		"GetInstanceData": func(context.Context, []any) ([]any, error) {
			if live.InstanceData == nil {
				return []any{nil}, nil
			}
			if data := live.InstanceData(); data != nil {
				return []any{data}, nil
			}
			return []any{nil}, nil
		},
		// SaveInstanceData no-ops: C++ calls the shim's SaveToDB,
		// which serializes the data table (mar_encode) into
		// instance.data; Go has no such serialization, so the table
		// persists in memory for the runtime's lifetime only.
		"SaveInstanceData": func(context.Context, []any) ([]any, error) {
			return nil, nil
		},
	}
	return &Object{Type: "Map", Fields: map[string]any{"MapId": mapID, "InstanceId": instanceID, "InWorld": true}, Methods: methods}
}

// BGSnapshot is the battleground state behind a Lua BattleGround object: the
// fields BattleGroundMethods.h reads from the live BattleGround*. The snapshot
// is taken at hook-fire time (the world engine builds it before Trigger, so no
// method re-reads battleground state while a hook runs under a per-map state
// lock). Winner is the C++ PvPTeamId (SharedDefines.h:3499-3501: Horde 0,
// Alliance 1, Neutral 2); the world engine translates the hook's Team
// argument (Alliance 0, Horde 1) into it, and non-END fires read Neutral,
// the C++ _winnerTeamId default. MinPlayers/MaxPlayers are the per-team
// counts doubled (BattlegroundMgr.cpp:505-508). MinLevel/MaxLevel are
// the instance's m_LevelMin/m_LevelMax: the banked bracket range when
// the pop banked one (SetBracket, Battleground.cpp:1792), else the
// template row's overall range. GetBonusHonorFromKillCount therefore
// reads the bracket max, as C++ does (Battleground.cpp:809).
type BGSnapshot struct {
	Name              string
	MinLevel          uint32
	MaxLevel          uint32
	MinPlayers        uint32
	MaxPlayers        uint32
	MinPlayersPerTeam uint32
	MaxPlayersPerTeam uint32
	Winner            uint32
	// Status is the BattlegroundStatus at snapshot time (Battleground.h:
	// NONE 0, WAIT_QUEUE 1, WAIT_JOIN 2, IN_PROGRESS 3, WAIT_LEAVE 4),
	// and EndTime is m_EndTime in milliseconds (Battleground.cpp:512-517:
	// 0 while running, TIME_TO_AUTOREMOVE at end, decremented by
	// _ProcessLeave). Both are snapshot by the hook fire point, never
	// re-read under a state lock (see bgScriptObject).
	Status  uint32
	EndTime uint32
	// BracketID is the BattlegroundBracketId (Battleground.h:270, the
	// PvPDifficulty.dbc RangeIndex SetBracket banks at creation,
	// Battleground.cpp:1791); the C++ ctor default is BG_BRACKET_ID_FIRST
	// (0), which is also what an unbanked snapshot reads.
	BracketID uint32
	Map       *Object
	// AliveAlliance/AliveHorde are GetAlivePlayersCountByTeam's counts
	// (Battleground.cpp:1737: m_Players members of the team, online and
	// alive, excluding Spirit of Redemption form); InvitedAlliance/
	// InvitedHorde are m_InvitedAlliance/m_InvitedHorde (Battleground.h:
	// 323-327, invitees not yet removed, entered or not); PlayersAlliance/
	// PlayersHorde are GetPlayersCountByTeam's m_Players counts. All six
	// come from the world engine's slot scan at snapshot time (see
	// bgScriptObject), never re-read under a state lock.
	AliveAlliance   uint32
	AliveHorde      uint32
	InvitedAlliance uint32
	InvitedHorde    uint32
	PlayersAlliance uint32
	PlayersHorde    uint32
}

// BGLive carries the snapshot behind a Lua BattleGround object. A nil Load
// or a false return means the battleground is unknown, and the methods read
// zero values.
type BGLive struct {
	Load func() (BGSnapshot, bool)
}

// NewBattleGroundObject builds the Lua BattleGround object surface for the
// BG_EVENT_* hooks: GetName/GetTypeId/GetMapId/GetInstanceId/GetMap/
// GetMinLevel/GetMaxLevel/GetMinPlayers/GetMaxPlayers/GetMinPlayersPerTeam/
// GetMaxPlayersPerTeam/GetWinner/GetBonusHonorFromKillCount/GetStatus/
// GetEndTime/GetBracketId/GetAlivePlayersCountByTeam/GetFreeSlotsForTeam
// (BattleGroundMethods.h). GetBonusHonorFromKillCount is the
// pure C++ formula (Battleground.cpp:806-811): hk_honor_at_level(min(maxLevel, 80),
// kills) = ceil(level * 1.55 * kills), without the OnHonorCalculation script
// hook, which has no Go twin. GetStatus/GetEndTime/GetBracketId read the pre-fire
// snapshot (see bgScriptObject), not live state. GetAlivePlayersCountByTeam
// takes the C++ Team domain (SharedDefines.h:797-802: ALLIANCE 469, HORDE 67),
// not TeamId 0/1: any other argument matches no member and reads 0, like the
// C++ exact Team comparison against BattlegroundPlayer.Team. GetFreeSlotsForTeam
// ports Battleground.cpp:1126-1196 under the default Battleground.InvitationType
// = 0 (NO_BALANCE, World.cpp:1205): WAIT_JOIN reads the no-balance leg
// (MaxPlayersPerTeam minus invited, clamped at 0) and IN_PROGRESS reads the
// balanced diff/diff2/diff3 formula; every other status reads 0. Its else
// branch treats any non-ALLIANCE argument as HORDE, like the C++.
// DELTAS: the snapshot counts come from the Go slot roster, not m_Players:
// offline members in C++'s 5-minute grace are absent (no session to scan),
// and Spirit of Redemption form is not excluded separately (Go death sets
// Health 0, so the alive gate already drops dead priests).
func NewBattleGroundObject(bgTypeID, mapID, instanceID uint32, live BGLive) *Object {
	load := live.Load
	if load == nil {
		load = func() (BGSnapshot, bool) { return BGSnapshot{}, false }
	}
	methods := map[string]ObjectMethod{
		"GetName": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.Name}, nil
			}
			return []any{nil}, nil
		},
		"GetTypeId": func(context.Context, []any) ([]any, error) { return []any{bgTypeID}, nil },
		"GetMapId":  func(context.Context, []any) ([]any, error) { return []any{mapID}, nil },
		"GetInstanceId": func(context.Context, []any) ([]any, error) {
			return []any{instanceID}, nil
		},
		"GetMap": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok && snap.Map != nil {
				return []any{snap.Map}, nil
			}
			return []any{nil}, nil
		},
		"GetMinLevel": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.MinLevel}, nil
			}
			return []any{uint32(0)}, nil
		},
		"GetMaxLevel": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.MaxLevel}, nil
			}
			return []any{uint32(0)}, nil
		},
		"GetMinPlayers": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.MinPlayers}, nil
			}
			return []any{uint32(0)}, nil
		},
		"GetMaxPlayers": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.MaxPlayers}, nil
			}
			return []any{uint32(0)}, nil
		},
		"GetMinPlayersPerTeam": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.MinPlayersPerTeam}, nil
			}
			return []any{uint32(0)}, nil
		},
		"GetMaxPlayersPerTeam": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.MaxPlayersPerTeam}, nil
			}
			return []any{uint32(0)}, nil
		},
		"GetWinner": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.Winner}, nil
			}
			return []any{uint32(2)}, nil
		},
		"GetStatus": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.Status}, nil
			}
			return []any{uint32(0)}, nil
		},
		"GetEndTime": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.EndTime}, nil
			}
			return []any{uint32(0)}, nil
		},
		"GetBracketId": func(context.Context, []any) ([]any, error) {
			if snap, ok := load(); ok {
				return []any{snap.BracketID}, nil
			}
			return []any{uint32(0)}, nil
		},
		"GetBonusHonorFromKillCount": func(_ context.Context, args []any) ([]any, error) {
			kills, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			snap, _ := load()
			level := snap.MaxLevel
			if level > 80 {
				level = 80
			}
			return []any{uint32(math.Ceil(float64(level) * 1.55 * float64(kills)))}, nil
		},
		"GetAlivePlayersCountByTeam": func(_ context.Context, args []any) ([]any, error) {
			team, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			snap, _ := load()
			switch team {
			case 469:
				return []any{snap.AliveAlliance}, nil
			case 67:
				return []any{snap.AliveHorde}, nil
			default:
				return []any{uint32(0)}, nil
			}
		},
		"GetFreeSlotsForTeam": func(_ context.Context, args []any) ([]any, error) {
			team, err := objectArgumentUint32(args, 0)
			if err != nil {
				return nil, err
			}
			snap, _ := load()
			var thisInvited, otherInvited, thisPlayers, otherPlayers uint32
			if team == 469 {
				thisInvited, otherInvited = snap.InvitedAlliance, snap.InvitedHorde
				thisPlayers, otherPlayers = snap.PlayersAlliance, snap.PlayersHorde
			} else {
				thisInvited, otherInvited = snap.InvitedHorde, snap.InvitedAlliance
				thisPlayers, otherPlayers = snap.PlayersHorde, snap.PlayersAlliance
			}
			switch snap.Status {
			case 2:
				if thisInvited < snap.MaxPlayersPerTeam {
					return []any{snap.MaxPlayersPerTeam - thisInvited}, nil
				}
				return []any{uint32(0)}, nil
			case 3:
				var diff uint32
				if otherInvited == thisInvited {
					diff = 1
				} else if otherInvited > thisInvited {
					diff = otherInvited - thisInvited
				}
				diff2 := uint32(0)
				if thisInvited < snap.MaxPlayersPerTeam {
					diff2 = snap.MaxPlayersPerTeam - thisInvited
				}
				var diff3 uint32
				if otherPlayers == thisPlayers {
					diff3 = 1
				} else if otherPlayers > thisPlayers {
					diff3 = otherPlayers - thisPlayers
				} else if thisInvited <= snap.MinPlayersPerTeam {
					diff3 = snap.MinPlayersPerTeam - thisInvited + 1
				}
				if diff2 < diff {
					diff = diff2
				}
				if diff3 < diff {
					diff = diff3
				}
				return []any{diff}, nil
			default:
				return []any{uint32(0)}, nil
			}
		},
	}
	return &Object{Type: "BattleGround", Fields: map[string]any{"MapId": mapID, "InstanceId": instanceID, "InWorld": true}, Methods: methods}
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
