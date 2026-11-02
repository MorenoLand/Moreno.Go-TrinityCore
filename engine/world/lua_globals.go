package world

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// luaGameEventIDs is the GameEventProvider behind the Eluna
// GetActiveGameEvents / IsGameEventActive globals: the cached active event
// set (conditions.go, refreshed at most once a minute), ascending like the
// C++ GameEventMgr::ActiveEvents std::set iteration order.
func (s *Server) luaGameEventIDs() []uint32 {
	if s == nil {
		return nil
	}
	active := s.cachedActiveGameEvents(context.Background())
	ids := make([]uint32, 0, len(active))
	for id := range active {
		ids = append(ids, uint32(id))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// luaMapByID is the MapProvider behind the Eluna GetMapById global: the
// Map object for a known map row, nil when the row is missing (the C++
// MapMgr::FindMap nullptr arm) or without a data store. The instance id is
// carried on the object; Go has no per-instance map registry to check it
// against, so an unknown instance still yields the map object.
func (s *Server) luaMapByID(mapID, instanceID uint32) *scripting.Object {
	if s == nil || s.Data == nil {
		return nil
	}
	if _, found, err := s.Data.Map(mapID); err != nil || !found {
		return nil
	}
	return scripting.NewMapObject(mapID, instanceID, s.mapLive(mapID, instanceID))
}

// luaWorldMessage is the WorldMessageProvider behind the Eluna
// SendWorldMessage global: World::SendServerMessage(SERVER_MSG_STRING,
// message) (World.cpp:3074), bridged by broadcastServerMessageAll with the
// raw message, not the .announce LANG_SYSTEMMESSAGE wrap.
func (s *Server) luaWorldMessage(message string) {
	if s == nil {
		return
	}
	s.broadcastServerMessageAll(serverMessageString, message)
}

// luaAreaName is the AreaNameProvider behind the Eluna GetAreaName
// global: the AreaTable.dbc name for the id, unknown ids read not-found
// (the C++ sAreaTableStore null arm).
func (s *Server) luaAreaName(id uint32) (string, bool) {
	if s == nil || s.Data == nil {
		return "", false
	}
	area, found, err := s.Data.Area(id)
	if err != nil || !found {
		return "", false
	}
	return area.Name, true
}

// luaSpawnMask is the SaveToDB spawnMask for a PerformIngameSpawn saved
// arm: 1 << map spawn mode, where the Go instance table's difficulty is
// the spawn mode; non-instance maps and missing rows read mode 0.
func (s *Server) luaSpawnMask(instanceID uint32) uint32 {
	if instanceID == 0 || s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return 1
	}
	var difficulty int64
	if err := s.CharactersStore.DB.QueryRowContext(context.Background(),
		"SELECT difficulty FROM instance WHERE id = ?", instanceID).Scan(&difficulty); err != nil || difficulty < 0 || difficulty > 30 {
		return 1
	}
	return 1 << uint(difficulty)
}

// luaPerformIngameSpawn is the SpawnProvider behind the Eluna
// PerformIngameSpawn global (GlobalMethods.h, Trinity branch). The map
// check is the FindMap nullptr arm (the luaMapByID row check; Go has no
// per-instance map registry, so an unknown instance still spawns into
// the map's instance bucket). Creature unsaved reuses summonTempCreature
// with the Map::SummonCreature duration semantics: durorresptime is
// milliseconds straight into the InitStats timer, and the type override
// after the summon is TIMED_OR_DEAD_DESPAWN (0) when it is positive,
// MANUAL_DESPAWN (7) otherwise. Creature saved mirrors the .npc add row
// (MAX(guid)+1, spawntimesecs 120) with the caller's phase and the
// instance spawn mask, then materializes a live motion like
// LoadFromDB + map->Add; the Trinity saved arm has no ForcedDespawn, so
// durorresptime is ignored there. Gameobject saved mirrors the .gobject
// add row with the caller's phase and durorresptime as spawntimesecs
// (SetRespawnTime takes seconds on this arm); gameobject unsaved mirrors
// the SummonGameObject dynamic spawn with durorresptime seconds as the
// despawn timer. DELTAS: the Trinity displayId DBC check on the
// gameobject arm has no Go counterpart (the SummonGameObject path skips
// it too), so only a missing template reads nil; a saved gameobject is
// broadcast like a fresh .gobject add rather than waiting for grid load.
func (s *Server) luaPerformIngameSpawn(args scripting.SpawnArgs) *scripting.Object {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || s.Data == nil {
		return nil
	}
	if _, found, err := s.Data.Map(args.MapID); err != nil || !found {
		return nil
	}
	ctx := context.Background()
	switch args.SpawnType {
	case 1:
		var exists int64
		if err := s.WorldStore.DB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM creature_template WHERE entry = ?", args.Entry).Scan(&exists); err != nil || exists == 0 {
			return nil
		}
		if args.Save {
			return s.luaSpawnSavedCreature(ctx, args)
		}
		summonType := uint8(7)
		if args.DurOrRespawnTime > 0 {
			summonType = 0
		}
		origin := tempSummonOrigin{Map: args.MapID, InstanceID: args.InstanceID,
			X: args.X, Y: args.Y, Z: args.Z, O: args.O, Phase: args.Phase, HasPhase: true}
		motion := s.summonTempCreature(ctx, origin, args.Entry, args.X, args.Y, args.Z, args.O, summonType, args.DurOrRespawnTime)
		if motion == nil {
			return nil
		}
		return s.luaMotionCreature(motion)
	case 2:
		tmpl, err := gobjectTemplateByEntry(ctx, s.WorldStore.DB, args.Entry)
		if err != nil || tmpl == nil {
			return nil
		}
		if args.Save {
			return s.luaSpawnSavedGameObject(ctx, args, tmpl)
		}
		return s.luaSpawnRuntimeGameObject(args, tmpl)
	default:
		return nil
	}
}

// luaSpawnSavedCreature is the PerformIngameSpawn creature-save arm.
func (s *Server) luaSpawnSavedCreature(ctx context.Context, args scripting.SpawnArgs) *scripting.Object {
	var guid uint32
	if err := s.WorldStore.DB.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(guid), 0) + 1 FROM creature").Scan(&guid); err != nil {
		return nil
	}
	if _, err := s.WorldStore.DB.ExecContext(ctx,
		"INSERT INTO creature (guid, id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, spawntimesecs, MovementType) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 120, 0)",
		guid, args.Entry, args.MapID, s.luaSpawnMask(args.InstanceID), args.Phase, args.X, args.Y, args.Z, args.O); err != nil {
		return nil
	}
	var walkSpeed, runSpeed float64 = 1.0, 1.14286
	_ = s.WorldStore.DB.QueryRowContext(ctx,
		"SELECT COALESCE(speed_walk, 1), COALESCE(speed_run, 1) FROM creature_template WHERE entry = ?",
		args.Entry).Scan(&walkSpeed, &runSpeed)
	motion := s.motionFor(ctx, guid, args.Entry, args.MapID, args.InstanceID, args.X, args.Y, args.Z, args.O, 0, 0, float32(walkSpeed), float32(runSpeed), 0)
	if motion == nil {
		return nil
	}
	s.motionMu.Lock()
	motion.PhaseMask = args.Phase
	s.motionMu.Unlock()
	stats := s.loadCreatureStats(ctx, args.Entry)
	spawn := creatureSpawn{
		GUID: guid, RawGUID: motion.GUID, Entry: args.Entry, Map: args.MapID,
		X: args.X, Y: args.Y, Z: args.Z, Orientation: args.O, PhaseMask: args.Phase,
		Model: stats.Model, Faction: motion.Faction, NPCFlags: stats.NPCFlags,
		UnitFlags: stats.UnitFlags, Level: stats.Level,
		Health: stats.Health, MaxHealth: stats.MaxHealth,
		Scale: stats.Scale, Mana: stats.Mana,
		FlagsExtra: stats.FlagsExtra, TypeFlags: stats.TypeFlags,
		WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed),
	}
	s.applyCreatureMotionToSpawn(playerState{Map: args.MapID, InstanceID: args.InstanceID}, &spawn)
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildCreatureUpdate(ctx, spawn, nil))
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		s.broadcastCreatureCreateToVisibleSet(packet.Opcode, packet.Payload.Bytes(), args.MapID, args.InstanceID, spawn.X, spawn.Y, spawn.Z, &spawn, nil)
	}
	return s.luaMotionCreature(motion)
}

// luaSpawnSavedGameObject is the PerformIngameSpawn gameobject-save arm.
func (s *Server) luaSpawnSavedGameObject(ctx context.Context, args scripting.SpawnArgs, tmpl *gobjectTemplateRow) *scripting.Object {
	rotX, rotY, rotZ, rotW := gobjectQuatFromEulerZYX(float64(args.O), 0, 0)
	res, err := s.WorldStore.DB.ExecContext(ctx, `INSERT INTO gameobject (id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, rotation0, rotation1, rotation2, rotation3, spawntimesecs, animprogress, state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 255, 1)`,
		args.Entry, args.MapID, s.luaSpawnMask(args.InstanceID), args.Phase, args.X, args.Y, args.Z, args.O, rotX, rotY, rotZ, rotW, args.DurOrRespawnTime)
	if err != nil {
		return nil
	}
	spawnID, _ := res.LastInsertId()
	size := tmpl.size
	if size <= 0 {
		size = 1.0
	}
	spawn := gameObjectSpawn{
		GUID: uint32(spawnID), Entry: args.Entry, Map: args.MapID,
		X: args.X, Y: args.Y, Z: args.Z, Orientation: args.O,
		RotationX: rotX, RotationY: rotY, RotationZ: rotZ, RotationW: rotW,
		State: GameObjectStateReady, Type: tmpl.goType, DisplayID: tmpl.displayID,
		Size: size, ParentRotation: [4]float32{0, 0, 0, 1}, PhaseMask: args.Phase,
	}
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildGameObjectUpdate(spawn))
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		s.broadcastGameObjectCreateToVisibleSetAt(packet.Opcode, packet.Payload.Bytes(), args.MapID, args.InstanceID, args.X, args.Y, args.Z, args.Phase)
	}
	state, ok := s.loadLuaGameObject(ctx, uint32(spawnID), args.Entry)
	if !ok {
		return nil
	}
	state.InstanceID = args.InstanceID
	return s.luaGameObjectObject(state)
}

// luaSpawnRuntimeGameObject is the PerformIngameSpawn gameobject-unsaved
// arm: the SummonGameObject dynamic spawn on the caller's map, instance,
// and phase, with durorresptime seconds as the despawn timer.
func (s *Server) luaSpawnRuntimeGameObject(args scripting.SpawnArgs, tmpl *gobjectTemplateRow) *scripting.Object {
	lowGUID := s.nextDynamicGameObjectLowGUID()
	newGUID := gameObjectGUID(lowGUID, args.Entry)
	size := tmpl.size
	if size <= 0 {
		size = 1.0
	}
	dyn := &dynamicGameObjectState{GUID: newGUID, LowGUID: lowGUID, Entry: args.Entry, Map: args.MapID, InstanceID: args.InstanceID, X: args.X, Y: args.Y, Z: args.Z, Orientation: args.O, State: GameObjectStateReady, Type: tmpl.goType, DisplayID: tmpl.displayID, Size: size, ParentRotation: [4]float32{0, 0, 0, 1}, PhaseMask: args.Phase, IsRuntimeSpawn: true}
	if args.DurOrRespawnTime > 0 {
		dyn.DespawnTimer = time.AfterFunc(time.Duration(args.DurOrRespawnTime)*time.Second, func() {
			s.despawnDynamicGameObjectInInstance(dyn.Map, dyn.InstanceID, dyn.GUID)
		})
	}
	s.spawnDynamicGameObject(dyn)
	state := luaGameObjectState{GUID: newGUID, LowGUID: lowGUID, Entry: args.Entry, DisplayID: tmpl.displayID, Name: tmpl.name, Map: args.MapID, InstanceID: args.InstanceID, X: args.X, Y: args.Y, Z: args.Z, Orientation: args.O, Type: uint32(tmpl.goType), GoState: 1, LootState: 1, PhaseMask: args.Phase}
	return s.luaGameObjectObject(state)
}

// luaSendMail is the SendMailProvider behind the Eluna SendMail global
// (GlobalMethods.h): MailDraft::SendMailTo with MAIL_NORMAL, the caller's
// stationery, MAIL_CHECK_MASK_NONE, and deliver_time = now + delay. The
// item pairs are validated before anything is written (the C++
// luaL_error arms abort before the transaction commits, so a bad pair
// persists nothing either way): unknown entries and out-of-MaxCount
// amounts error, mirroring Item::CreateItem + the MaxCount gate. Items
// are minted like the recovered-item mail (generateItemGUID +
// item_instance with the template Duration, owner = receiver). Expiry
// is the SendMailTo rule via mailSendExpireDelay: COD 3 days, else a
// game-master sender 90 days, else 30 days, counted from delivery. The
// receiver row is written whether or not the character is online (the
// Eluna overload skips the receiver-existence gate); an online receiver
// gets the usual arrival notify. Without a store the call is a no-op,
// like the other provider-less globals.
func (s *Server) luaSendMail(args scripting.SendMailArgs) error {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return nil
	}
	ctx := context.Background()
	type mintedItem struct {
		entry  uint32
		amount uint32
	}
	minted := make([]mintedItem, 0, len(args.Items))
	for _, pair := range args.Items {
		var maxCount int64
		if err := s.WorldStore.DB.QueryRowContext(ctx,
			"SELECT COALESCE(maxcount, 0) FROM item_template WHERE entry = ?", pair.Entry).Scan(&maxCount); err != nil {
			return fmt.Errorf("Item entry %d does not exist", pair.Entry)
		}
		if pair.Amount < 1 || (maxCount > 0 && int64(pair.Amount) > maxCount) {
			return fmt.Errorf("Item entry %d has invalid amount %d", pair.Entry, pair.Amount)
		}
		minted = append(minted, mintedItem{entry: pair.Entry, amount: pair.Amount})
	}
	now := time.Now().Unix()
	deliver := now + int64(args.Delay)
	senderGM := false
	if sess := s.findSessionByGUID(uint64(args.SenderLow)); sess != nil {
		senderGM = sessionIsGameMaster(sess)
	}
	expire := deliver + mailSendExpireDelay(senderGM, args.COD)
	nextMailID := s.generateMailID()
	hasItems := 0
	if len(minted) > 0 {
		hasItems = 1
	}
	if _, err := s.CharactersStore.DB.ExecContext(ctx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
		VALUES (?, 0, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		nextMailID, args.Stationery, args.SenderLow, args.ReceiverLow, args.Subject, args.Text, hasItems, expire, deliver, args.Money, args.COD); err != nil {
		return err
	}
	for _, item := range minted {
		nextItemGUID := int64(s.generateItemGUID())
		if nextItemGUID <= 0 {
			continue
		}
		if _, err := s.CharactersStore.DB.ExecContext(ctx,
			"INSERT INTO item_instance (guid, itemEntry, owner_guid, count, duration) VALUES (?, ?, ?, ?, ?)",
			nextItemGUID, item.entry, args.ReceiverLow, item.amount, s.itemTemplateDuration(ctx, item.entry)); err != nil {
			continue
		}
		_, _ = s.CharactersStore.DB.ExecContext(ctx,
			"INSERT INTO mail_items (mail_id, item_guid, receiver) VALUES (?, ?, ?)",
			nextMailID, nextItemGUID, args.ReceiverLow)
	}
	s.notifyMailAvailable(uint64(args.ReceiverLow))
	return nil
}
