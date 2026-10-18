package world

import (
	"context"
	"database/sql"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	unitNPCFlagFlightmaster uint32 = 0x00002000
	playerExtraTaxiCheat    uint32 = 0x00000008
	taxiMaskSize                   = 14
)

// TrinityCore Player.h extra flags and transient PLAYER_FLAGS bits.
const (
	playerExtraGMOn            uint32 = 0x00000001
	playerExtraGMInvisible     uint32 = 0x00000010
	playerExtraGMChat          uint32 = 0x00000020
	playerFlagAFK              uint32 = 0x00000002
	playerFlagDND              uint32 = 0x00000004
	playerFlagGM               uint32 = 0x00000008
	playerFlagGroupLeader      uint32 = 0x00000001
	playerFlagAllowOnlyAbility uint32 = 0x00800000
)

// setTaxiMaskNode mirrors PlayerTaxi::SetTaximaskNode; returns true when the
// bit transitioned from unknown to known.
func (s *session) setTaxiMaskNode(node uint32) bool {
	if s.player == nil || node == 0 {
		return false
	}
	field := (node - 1) / 32
	if field >= taxiMaskSize {
		return false
	}
	bit := uint32(1) << ((node - 1) % 32)
	if s.player.TaxiMask[field]&bit != 0 {
		return false
	}
	s.player.TaxiMask[field] |= bit
	return true
}

func (s *session) isTaxiMaskNodeKnown(node uint32) bool {
	if s.player == nil || node == 0 {
		return false
	}
	field := (node - 1) / 32
	if field >= taxiMaskSize {
		return false
	}
	return s.player.TaxiMask[field]&(1<<((node-1)%32)) != 0
}

// isTaxiCheater mirrors Player::isTaxiCheater (PLAYER_EXTRA_TAXICHEAT).
func (s *session) isTaxiCheater() bool {
	return s.player != nil && s.player.ExtraFlags&playerExtraTaxiCheat != 0
}

// saveTaxiMask persists the mask in TrinityCore's space separated text form.
func (s *session) saveTaxiMask(ctx context.Context) {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.player == nil {
		return
	}
	parts := make([]string, taxiMaskSize)
	for i, v := range s.player.TaxiMask {
		parts[i] = strconv.FormatUint(uint64(v), 10)
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET taximask = ? WHERE guid = ?", strings.Join(parts, " "), s.playerGUID)
}

// loadTaxiMask mirrors PlayerTaxi::LoadTaxiMask (space separated words).
func (s *session) loadTaxiMask(raw sql.NullString) {
	if s == nil || s.player == nil || !raw.Valid {
		return
	}
	tokens := strings.Fields(raw.String)
	var validMask [taxiMaskSize]uint32
	if s.server != nil && s.server.Data != nil {
		if mask, err := s.server.Data.TaxiMask(); err == nil {
			validMask = mask
		}
	}
	for i := 0; i < taxiMaskSize && i < len(tokens); i++ {
		if v, err := strconv.ParseUint(tokens[i], 10, 32); err == nil {
			value := uint32(v)
			if validMask != [taxiMaskSize]uint32{} {
				value &= validMask[i]
			}
			s.player.TaxiMask[i] = value
		} else {
			s.player.TaxiMask[i] = 0
		}
	}
}

// nearestCreatureTaxiNode mirrors WorldSession::SendTaxiStatus
// (TaxiHandler.cpp:23-45): a creature that exists, is not hostile to the
// player, and carries the flightmaster flag yields its nearest taxi node —
// C++ applies no interact-range gate here (ObjectAccessor::GetCreature has
// no distance check), so the 5-yard canInteractWithNPC check must not run
// on this path. Callers that need the interact gate (taxi activation, menu)
// apply it themselves first, exactly like the C++ handlers do.
func (s *session) nearestCreatureTaxiNode(ctx context.Context, guid uint64) (uint32, bool) {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0, false
	}
	creature := s.luaCreature(ctx, guid)
	if creature == nil {
		return 0, false
	}
	if objectUint32OrZero(creature, "NPCFlags")&unitNPCFlagFlightmaster == 0 {
		return 0, false
	}
	// TrinityCore SendTaxiStatus: a hostile flight master gets no answer
	// (TaxiHandler.cpp:28: unit->IsHostileTo(player)), and C++ applies no
	// interact-range gate on this path.
	low := uint32(guid & 0x00FFFFFF)
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	var faction int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(t.faction, 0) FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?`, low, entry).Scan(&faction); err == nil {
		player := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.effectiveFactionTemplate(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
		if s.server.isHostileFaction(uint32(faction), player) {
			return 0, false
		}
	}
	mapID := objectUint32OrZero(creature, "Map")
	x, okX := objectFloat32Field(creature, "X")
	y, okY := objectFloat32Field(creature, "Y")
	z, okZ := objectFloat32Field(creature, "Z")
	if !okX || !okY || !okZ || s.server.Data == nil {
		return 0, false
	}
	node, err := s.server.Data.NearestTaxiNode(x, y, z, mapID, s.playerAlliance())
	if err != nil {
		return 0, false
	}
	return node, node != 0
}

// playerAlliance reports the player team like Player::GetTeam.
func (s *session) playerAlliance() bool {
	if s.player == nil {
		return false
	}
	switch s.player.Race {
	case 1, 3, 4, 7, 11: // Human, Dwarf, NightElf, Gnome, Draenei
		return true
	}
	return false
}

func (s *session) handleTaxiNodeStatusQuery(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return true
	}
	// TaxiHandler.cpp:23-45 (SendTaxiStatus): the status query answers for
	// any existing, non-hostile flightmaster — no interact-range gate.
	node, ok := s.nearestCreatureTaxiNode(ctx, guid)
	if !ok {
		return true
	}
	known := uint8(0)
	if s.isTaxiMaskNodeKnown(node) {
		known = 1
	}
	packet := protocol.NewBuffer(9)
	packet.WriteU64(guid)
	packet.WriteU8(known)
	_ = s.write(uint16(protocol.OpcodeSMSG_TAXINODE_STATUS), packet.Bytes(), true)
	return true
}

func (s *session) sendTaxiNodeStatusMultiple(ctx context.Context) bool {
	if s == nil || s.server == nil || s.player == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.Data == nil {
		return true
	}
	distance := float64(s.server.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150
	}
	var selectArgs []any
	npcFlagExpr := "0"
	if flagClause, ok := s.server.gameEventNPCFlagClause(ctx, &selectArgs); ok {
		npcFlagExpr = flagClause
	}
	var eventArgs []any
	eventClause := gameEventSpawnClause("gec.eventEntry", s.server.activeEventList(ctx), &eventArgs)
	isGM := s.player.ExtraFlags&playerExtraGMOn != 0 || s.player.PlayerFlags&playerFlagGM != 0
	// Serverside-hidden visibility rides on the GM visibility detect, not
	// GM_ON (see creatures.go): a GM after `.gm visible on` keeps
	// phase-anywhere but hidden flight masters re-hide.
	seesHidden := s.gmVisibilityDetect != 0
	isGhost := s.player.Health > 0 && s.player.PlayerFlags&playerFlagGhost != 0
	phaseMask := s.currentPlayerPhaseMask()
	// Runtime 261-phase overrides (AuraEffect::HandlePhase creature arm,
	// SpellAuraEffects.cpp:1581-1632): the query only sees the spawn's
	// static c.phaseMask row, so spawn GUIDs whose override intersects the
	// player's phase mask are OR'd into the phase predicate.
	taxiPhaseClause, taxiPhaseArgs := creaturePhasePredicate("? OR (c.phaseMask & ?) <> 0", s.server.creaturePhaseOverrideGUIDs(s.player.Map, phaseMask))
	query := `SELECT c.guid, c.id, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), (t.npcflag | ` + npcFlagExpr + `)
		FROM creature AS c JOIN creature_template AS t ON t.entry = c.id
		LEFT JOIN game_event_creature AS gec ON gec.guid = c.guid
		WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
		AND (` + taxiPhaseClause + `)
		AND (? OR ? OR ((COALESCE(t.flags_extra, 0) & 0x400) = 0 AND (COALESCE(t.npcflag, 0) & 0xC000) = 0))
		AND ` + eventClause + ` ORDER BY c.guid`
	queryArgs := make([]any, 0, len(selectArgs)+10+len(eventArgs)+len(taxiPhaseArgs))
	queryArgs = append(queryArgs, selectArgs...)
	queryArgs = append(queryArgs, s.player.Map, float64(s.player.X)-distance, float64(s.player.X)+distance, float64(s.player.Y)-distance, float64(s.player.Y)+distance, isGM, phaseMask)
	queryArgs = append(queryArgs, taxiPhaseArgs...)
	queryArgs = append(queryArgs, seesHidden, isGhost)
	queryArgs = append(queryArgs, eventArgs...)
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return true
	}
	type flightmaster struct {
		guid, entry uint32
		x, y, z     float32
		faction     uint32
	}
	masters := make([]flightmaster, 0)
	for rows.Next() {
		var guid, entry, faction, npcFlags int64
		var x, y, z float64
		if rows.Scan(&guid, &entry, &x, &y, &z, &faction, &npcFlags) == nil && uint32(npcFlags)&unitNPCFlagFlightmaster != 0 && validMovementPosition(float32(x), float32(y), float32(z), 0) && math.Sqrt((x-float64(s.player.X))*(x-float64(s.player.X))+(y-float64(s.player.Y))*(y-float64(s.player.Y))+(z-float64(s.player.Z))*(z-float64(s.player.Z))) <= distance {
			masters = append(masters, flightmaster{guid: uint32(guid), entry: uint32(entry), x: float32(x), y: float32(y), z: float32(z), faction: uint32(faction)})
		}
	}
	rows.Close()
	if rows.Err() != nil {
		return false
	}
	player := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.effectiveFactionTemplate(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	for _, master := range masters {
		if s.server.isHostileFaction(master.faction, player) {
			continue
		}
		node, nodeErr := s.server.Data.NearestTaxiNode(master.x, master.y, master.z, s.player.Map, s.playerAlliance())
		if nodeErr != nil || node == 0 {
			continue
		}
		packet := protocol.NewBuffer(9)
		packet.WriteU64(creatureWorldGUID(master.guid, master.entry))
		if s.isTaxiMaskNodeKnown(node) {
			packet.WriteU8(1)
		} else {
			packet.WriteU8(0)
		}
		if err := s.write(uint16(protocol.OpcodeSMSG_TAXINODE_STATUS), packet.Bytes(), true); err != nil {
			return false
		}
	}
	return true
}

func (s *session) handleTaxiQueryAvailableNodes(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return true
	}
	// TaxiHandler.cpp:69-74 (HandleTaxiQueryAvailableNodes): the flightmaster
	// interact gate fires first — a non-flightmaster GUID is silently
	// dropped, like the activate path.
	if !s.canInteractWithNPC(ctx, guid, uint64(unitNPCFlagFlightmaster)) {
		return true
	}
	// TaxiHandler.cpp:78-80 — the feign-death strip runs after the interact
	// gate, before the learn-new-node check.
	s.removeFakeDeath()
	// HandleTaxiQueryAvailableNodes: learn unknown node, otherwise show the
	// flight map for known nodes.
	if s.learnNewTaxiNode(ctx, guid) {
		return true
	}
	return s.sendTaxiMenu(ctx, guid)
}

// learnNewTaxiNode mirrors WorldSession::SendLearnNewTaxiNode: discovers the
// nearest node, stores it and notifies the client; true when learned. C++
// SendLearnNewTaxiNode applies no interact gate of its own (both callers —
// HandleTaxiQueryAvailableNodes and the gossip TAXIVENDOR arm — gate first),
// so the ungated lookup is the faithful one.
func (s *session) learnNewTaxiNode(ctx context.Context, guid uint64) bool {
	node, ok := s.nearestCreatureTaxiNode(ctx, guid)
	if !ok {
		return true // avoid a second failing node search in SendTaxiMenu
	}
	if !s.isTaxiMaskNodeKnown(node) && s.setTaxiMaskNode(node) {
		s.saveTaxiMask(ctx)
		_ = s.write(uint16(protocol.OpcodeSMSG_NEW_TAXI_PATH), nil, true)
		status := protocol.NewBuffer(9)
		status.WriteU64(guid)
		status.WriteU8(1)
		_ = s.write(uint16(protocol.OpcodeSMSG_TAXINODE_STATUS), status.Bytes(), true)
		return true
	}
	return false
}

func (s *session) sendTaxiMenu(ctx context.Context, flightMasterGUID uint64) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	creature := s.luaCreature(ctx, flightMasterGUID)
	if creature == nil {
		return true
	}
	if objectUint32OrZero(creature, "NPCFlags")&unitNPCFlagFlightmaster == 0 {
		return true
	}
	mapID := objectUint32OrZero(creature, "Map")
	x, _ := objectFloat32Field(creature, "X")
	y, _ := objectFloat32Field(creature, "Y")
	z, _ := objectFloat32Field(creature, "Z")
	curNode := uint32(0)
	if s.server.Data != nil {
		if node, err := s.server.Data.NearestTaxiNode(x, y, z, mapID, s.playerAlliance()); err == nil {
			curNode = node
		}
	}
	if curNode == 0 {
		return true
	}
	// SendTaxiMenu (TaxiHandler.cpp:103-131): Grimwing in Ebon Hold
	// (entry 29480) forces the taxi-cheater view for this menu only — the
	// player's own flag is restored right after the packet goes out.
	lastCheater := s.isTaxiCheater()
	if objectUint32OrZero(creature, "Entry") == 29480 && s.player != nil {
		s.player.ExtraFlags |= playerExtraTaxiCheat
	}
	// SendTaxiMenu never announces SMSG_NEW_TAXI_PATH; the map carries the
	// player's known nodes (or the whole network for taxi cheaters).
	packet := protocol.NewBuffer(4 + 8 + 4 + 8*taxiMaskSize)
	packet.WriteU32(1)
	packet.WriteU64(flightMasterGUID)
	packet.WriteU32(curNode)
	cheater := s.isTaxiCheater()
	networkMask := [taxiMaskSize]uint32{}
	if s.server.Data != nil {
		networkMask, _ = s.server.Data.TaxiNetworkMask()
	}
	for i := 0; i < taxiMaskSize; i++ {
		if cheater {
			packet.WriteU32(networkMask[i])
		} else {
			packet.WriteU32(s.player.TaxiMask[i])
		}
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_SHOWTAXINODES), packet.Bytes(), true)
	if s.player != nil && !lastCheater {
		s.player.ExtraFlags &^= playerExtraTaxiCheat
	}
	s.debug("taxi menu sent", "account", s.accountName, "master", flightMasterGUID, "node", curNode)
	return true
}

// ActivateTaxiReply codes from TrinityCore SharedDefines.h.
const (
	taxiErrOK             uint32 = 0
	taxiErrUnspecified    uint32 = 1
	taxiErrNoSuchPath     uint32 = 2
	taxiErrNotEnoughMoney uint32 = 3
	taxiErrTooFarAway     uint32 = 4
	taxiErrNoVendorNearby uint32 = 5
	taxiErrNotVisited     uint32 = 6
	taxiErrPlayerBusy     uint32 = 7 // ERR_TAXIPLAYERBUSY (SharedDefines.h)
	taxiErrAlreadyMounted uint32 = 8 // ERR_TAXIPLAYERALREADYMOUNTED (SharedDefines.h:3656)
	taxiErrShapeshifted   uint32 = 9 // ERR_TAXIPLAYERSHAPESHIFTED (SharedDefines.h:3657)
)

// taxiMountDisplayID mirrors ObjectMgr::GetTaxiMountDisplayId
// (ObjectMgr.cpp:6745-6782): the taxi node stores a mount creature ENTRY,
// resolved to a display id through the template's random valid model
// (CreatureTemplate::GetRandomValidModelId, Creature.cpp:112) with the 50%
// other-gender model swap (ObjectMgr::GetCreatureModelRandomGender,
// ObjectMgr.cpp:1613-1638). A missing template or a template with no valid
// model yields 0, like the C++ arm.
func (s *session) taxiMountDisplayID(ctx context.Context, node uint32, allowedAltTeam bool) uint32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0
	}
	entry, err := s.server.Data.TaxiNodeMountEntry(node, s.playerAlliance(), allowedAltTeam)
	if err != nil || entry == 0 {
		return 0
	}
	var m1, m2, m3, m4 uint32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT modelid1, modelid2, modelid3, modelid4 FROM creature_template WHERE entry = ?", entry).Scan(&m1, &m2, &m3, &m4); err != nil {
		return 0
	}
	models := make([]uint32, 0, 4)
	for _, m := range []uint32{m1, m2, m3, m4} {
		if m != 0 {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return 0
	}
	display := models[rand.Intn(len(models))]
	var otherGender uint32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT DisplayID_Other_Gender FROM creature_model_info WHERE DisplayID = ?", display).Scan(&otherGender); err == nil && otherGender != 0 && rand.Intn(2) == 0 {
		display = otherGender
	}
	return display
}

// handleActivateTaxi serves both CMSG_ACTIVATETAXI and
// CMSG_ACTIVATETAXIEXPRESS; express selects the packet layout. C++ keeps
// the two handlers separate (HandleActivateTaxiOpcode vs
// HandleActivateTaxiExpressOpcode, TaxiHandler.cpp:166-172/256-285), so a
// 16-byte EXPRESS packet (nodeCount=1) must NOT be misread as an
// ACTIVATETAXI source/dest pair — C++ answers it silently via
// ActivateTaxiPathTo's nodes.size() < 2 arm.
func (s *session) handleActivateTaxi(ctx context.Context, payload []byte, express bool) bool {
	minLen := 16
	if express {
		minLen = 12
	}
	if !s.playerLoaded || s.player == nil || len(payload) < minLen {
		return true
	}
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return true
	}
	reply := func(code uint32) bool {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(code)
		_ = s.write(uint16(protocol.OpcodeSMSG_ACTIVATETAXIREPLY), packet.Bytes(), true)
		return true
	}

	// C++ HandleActivateTaxiOpcode (TaxiHandler.cpp:259-264) and
	// HandleActivateTaxiExpressOpcode (:166-172): the flightmaster interact
	// gate fires FIRST — before any taximask check — and answers
	// ERR_TAXITOOFARAWAY (4) when the player can't interact with the NPC.
	if !s.canInteractWithNPC(ctx, guid, uint64(unitNPCFlagFlightmaster)) {
		return reply(taxiErrTooFarAway)
	}

	var nodes []uint32
	if express {
		// CMSG_ACTIVATETAXIEXPRESS: guid (8), nodeCount (4), nodes... (nodeCount * 4).
		nodeCount, err := reader.ReadU32()
		if err != nil || len(payload) < int(12+nodeCount*4) {
			return true // truncated express list: the C++ overread throws and the session drops with no reply
		}
		if nodeCount == 0 {
			return true // HandleActivateTaxiExpressOpcode: empty node list returns silently
		}
		for i := uint32(0); i < nodeCount; i++ {
			node, err := reader.ReadU32()
			if err != nil {
				return true // truncated node: drop, no reply
			}
			if !s.isTaxiMaskNodeKnown(node) && !s.isTaxiCheater() {
				return reply(taxiErrNotVisited)
			}
			nodes = append(nodes, node)
		}
		if len(nodes) < 2 {
			return true // Player::ActivateTaxiPathTo: nodes.size() < 2 returns false with no reply
		}
	} else {
		// CMSG_ACTIVATETAXI: guid (8), source (4), dest (4).
		// C++ HandleActivateTaxiOpcode (TaxiHandler.cpp:267-271) gates BOTH
		// endpoints on the taximask — an unvisited destination answers
		// ERR_TAXINOTVISITED too, not just an unvisited source.
		sourceNode, err := reader.ReadU32()
		if err != nil {
			return true
		}
		destNode, err := reader.ReadU32()
		if err != nil {
			return true
		}
		if !s.isTaxiCheater() && (!s.isTaxiMaskNodeKnown(sourceNode) || !s.isTaxiMaskNodeKnown(destNode)) {
			return reply(taxiErrNotVisited)
		}
		nodes = []uint32{sourceNode, destNode}
	}
	sourceNode := nodes[0]

	// Player::ActivateTaxiPathTo (Player.cpp:21517-21558): the pre-flight
	// state gates fire after the handler's interact/known-node gates, in
	// C++ order. Logging-out, in-combat, stunned or rooted answers
	// ERR_TAXIPLAYERBUSY (7); a possessed/mind-controlled player
	// (UNIT_FLAG_REMOVE_CLIENT_CONTROL, :21527-21528) drops silently;
	// mounted answers ERR_TAXIPLAYERALREADYMOUNTED (8); a shapeshifted
	// player answers ERR_TAXIPLAYERSHAPESHIFTED (9, :21535-21539); a
	// non-melee cast in progress answers ERR_TAXIPLAYERBUSY (7).
	// IsInDisallowedMountForm (Unit.cpp:9171) consults the
	// SpellShapeshiftForm DBC flags and the display chain — Go tracks only
	// the raw form id, so any active form answers 9, exact for every form
	// whose DBC flags lack the mount-allowed bit.
	if !s.logoutAt.IsZero() || s.isInCombat() || s.player.UnitFlags&unitFlagStunned != 0 || s.rooted {
		return reply(taxiErrPlayerBusy)
	}
	if s.player.UnitFlags&unitFlagRemoveClientControl != 0 {
		return true
	}
	if s.player.MountDisplayID != 0 {
		return reply(taxiErrAlreadyMounted)
	}
	if s.player.ShapeshiftForm != 0 {
		return reply(taxiErrShapeshifted)
	}
	if s.genericCastInProgress() {
		return reply(taxiErrPlayerBusy)
	}
	// No ERR_TAXINOVENDORNEARBY arm here: the enum value exists in
	// SharedDefines.h but no server path sends it — ActivateTaxiPathTo
	// never consults the flightmaster's own node, only the packet's
	// source node (Player.cpp:21562-21567).
	if s.server.Data == nil {
		return reply(taxiErrUnspecified)
	}
	// Player::ActivateTaxiPathTo (Player.cpp:21562-21567): an unknown source
	// node answers ERR_TAXINOSUCHPATH before any hop resolution.
	if !s.server.Data.TaxiNodeExists(sourceNode) {
		return reply(taxiErrNoSuchPath)
	}
	// Player::ActivateTaxiPathTo (Player.cpp:21591-21608): each consecutive
	// node pair resolves through its own direct TaxiPath link (the C++
	// ObjectMgr::GetTaxiPath lookup); a missing hop clears the destinations
	// and returns false with NO reply — silent, like the C++ arm.
	var pathIDs []uint32
	var hopPrices []uint64
	var rawTotal uint64
	for i := 1; i < len(nodes); i++ {
		pathID, hopPrice, found, herr := s.server.Data.TaxiPathLinks(nodes[i-1], nodes[i])
		if herr != nil || !found || pathID == 0 {
			return true
		}
		pathIDs = append(pathIDs, pathID)
		hopPrices = append(hopPrices, uint64(hopPrice))
		rawTotal += uint64(hopPrice)
	}
	// SendDoFlight mount gate (Player.cpp:21610-21616) fires BEFORE the
	// money gate: no mount model for the source node answers
	// ERR_TAXIUNSPECIFIEDSERVERERROR even when the player is also broke.
	// Player.cpp:21628-21634: the alternate-team mount fallback is allowed
	// only for the Acherus node (315) taken by a death knight — every other
	// npc-path flight resolves its own team's mount entry.
	mount := s.taxiMountDisplayID(ctx, sourceNode, sourceNode == 315 && s.player.Class == classDeathKnight)
	if mount == 0 {
		return reply(taxiErrUnspecified)
	}
	// Player::ActivateTaxiPathTo (Player.cpp:21624-21631, 21644-21651): the
	// reputation discount applies to the summed multi-hop cost and the money
	// gate runs against the discounted total — but the deduction itself is
	// per-leg: the first leg at departure, each later leg on arrival at its
	// hop (FlightPathMovementGenerator.cpp:117-128), every leg ceiled
	// individually (FlightPathMovementGenerator.cpp:224).
	discount := s.reputationPriceDiscount(ctx, guid)
	totalCost := uint32(math.Ceil(float64(rawTotal) * discount))
	if uint64(s.player.Money) < uint64(totalCost) {
		return reply(taxiErrNotEnoughMoney)
	}
	// Player::ActivateTaxiPathTo (Player.cpp:21580): the player leaves any
	// vehicle before the flight starts — Go's exitVehicle mirrors
	// Unit::_ExitVehicle and is a no-op when not mounted on a vehicle.
	s.exitVehicle(ctx)
	// Flight start tears the trade down: the client closes its trade window
	// when the taxi map opens, but cheating tools can reopen it, so C++
	// calls TradeCancel(true) unconditionally at flight start
	// (Player::ActivateTaxiPathTo, Player.cpp:21586).
	s.cancelTrade(true)
	// SendDoFlight (TaxiHandler.cpp:117-119) strips a faked death at flight
	// start, after the ActivateTaxiPathTo money deduction.
	s.removeFakeDeath()
	// SendDoFlight (Player.cpp:21683): C++ merges every hop into one
	// FlightPathMovementGenerator path (LoadPath) and chains the legs
	// server-side via path-switch events; the client only ever sees one
	// spline per map segment (GetPathAtMapEnd). beginTaxiFlight builds that
	// model — including the departure deduction of the first leg.
	previousTaxiPath := s.player.TaxiPath
	faction := s.taxiFlightMasterFaction(ctx, guid)
	s.player.TaxiPath = formatTaxiPath(faction, nodes)
	if !s.beginTaxiFlight(ctx, nodes, pathIDs, hopPrices, discount, faction, mount, 0, true) {
		s.player.TaxiPath = previousTaxiPath
	}
	s.updateAchievementCriteria(criteriaTypeFlightPathsTaken, 0, 1)
	s.debug("taxi flight activated", "account", s.accountName, "master", guid, "nodes", nodes, "cost", totalCost)
	return reply(taxiErrOK)
}

func (s *session) taxiFlightMasterFaction(ctx context.Context, guid uint64) uint32 {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0
	}
	var faction uint32
	low, entry := uint32(guid&0x00FFFFFF), uint32((guid>>24)&0x00FFFFFF)
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT t.faction FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?", low, entry).Scan(&faction); err != nil {
		return 0
	}
	return faction
}

func taxiResumeStartNode(points []wotlk.TaxiSplinePoint, mapID uint32, x, y, z float32) int {
	if len(points) < 2 {
		return 0
	}
	distNext := float64((points[0].X-x)*(points[0].X-x) + (points[0].Y-y)*(points[0].Y-y) + (points[0].Z-z)*(points[0].Z-z))
	for index := 1; index < len(points); index++ {
		point, previous := points[index], points[index-1]
		if point.MapID != int32(mapID) {
			continue
		}
		distPrevious := distNext
		distNext = float64((point.X-x)*(point.X-x) + (point.Y-y)*(point.Y-y) + (point.Z-z)*(point.Z-z))
		distNodes := float64((point.X-previous.X)*(point.X-previous.X) + (point.Y-previous.Y)*(point.Y-previous.Y) + (point.Z-previous.Z)*(point.Z-previous.Z))
		if distNext+distPrevious < distNodes {
			return index
		}
	}
	return 0
}

func (s *session) continueTaxiFlight() {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	// PlayerTaxi::SaveTaxiDestinationsToString: "faction n0 n1 ..."; the
	// hop-boundary rewrites keep nodes[0] as the current source, so a relog
	// resumes the current hop with the remaining chain intact — the C++
	// LoadTaxiDestinationsFromString + ContinueTaxiFlight pair.
	nodes := strings.Fields(s.player.TaxiPath)
	if len(nodes) < 3 {
		return
	}
	faction, factionErr := strconv.ParseUint(nodes[0], 10, 32)
	if factionErr != nil {
		return
	}
	nodeIDs := make([]uint32, 0, len(nodes)-1)
	for _, token := range nodes[1:] {
		node, nodeErr := strconv.ParseUint(token, 10, 32)
		if nodeErr != nil {
			return
		}
		nodeIDs = append(nodeIDs, uint32(node))
	}
	if len(nodeIDs) < 2 {
		return
	}
	var pathIDs []uint32
	var hopPrices []uint64
	for i := 1; i < len(nodeIDs); i++ {
		pathID, hopPrice, found, err := s.server.Data.TaxiPathLinks(nodeIDs[i-1], nodeIDs[i])
		if err != nil || !found || pathID == 0 {
			return
		}
		pathIDs = append(pathIDs, pathID)
		hopPrices = append(hopPrices, uint64(hopPrice))
	}
	// Player::ContinueTaxiFlight (Player.cpp:21720-21728) resolves the mount
	// with the wide alternate-team lookup on the current source.
	mount := s.taxiMountDisplayID(context.Background(), nodeIDs[0], true)
	if mount == 0 {
		return
	}
	// FlightPathMovementGenerator::LoadPath recomputes the discount from the
	// stored flight-master faction template; the current hop was already
	// charged before the logout, so remaining legs charge on arrival.
	discount := s.reputationDiscountForFactionTemplate(uint32(faction))
	points, err := s.server.Data.TaxiPathPoints(pathIDs[0])
	if err != nil || len(points) == 0 {
		return
	}
	resumePt := taxiResumeStartNode(points, s.player.Map, s.player.X, s.player.Y, s.player.Z)
	previousTaxiPath := s.player.TaxiPath
	if !s.beginTaxiFlight(context.Background(), nodeIDs, pathIDs, hopPrices, discount, uint32(faction), mount, resumePt, false) {
		s.player.TaxiPath = previousTaxiPath
	}
}

// taxiSplineIDSource hands out SMSG_MONSTER_MOVE spline ids; the client
// echoes the id in CMSG_MOVE_SPLINE_DONE so the ack can be matched to the
// flight segment it closes.
var taxiSplineIDSource uint32

// taxiHop is one resolved node-pair leg of a taxi flight. It mirrors a
// FlightPathMovementGenerator::_pointsForPathSwitch entry
// (FlightPathMovementGenerator.cpp:224): the raw (pre-discount) hop price
// plus the merged-spline bookkeeping LoadPath builds around it.
type taxiHop struct {
	pathID   uint32
	rawPrice uint64
	endDist  float64 // cumulative merged-spline distance at this hop's last vertex
	endPt    int     // merged-point index of this hop's last vertex
}

// taxiSegment is one map-uniform run of the merged spline: the C++
// GetPathAtMapEnd() slice (FlightPathMovementGenerator.cpp:184-196). Each
// segment gets its own client spline; the client acks each with
// CMSG_MOVE_SPLINE_DONE and the server far-teleports between segments
// (HandleMoveSplineDoneOpcode, TaxiHandler.cpp:214-233).
type taxiSegment struct {
	startPt, endPt int // [startPt, endPt) into taxiFlight.points
}

// taxiFlight is the live multi-hop flight model: the C++ m_taxi destination
// deque (nodes) plus the merged FlightPathMovementGenerator path (points).
type taxiFlight struct {
	faction   uint32
	mount     uint32
	discount  float64
	nodes     []uint32 // remaining destinations; nodes[0] is the current source
	hops      []taxiHop
	points    []wotlk.TaxiSplinePoint
	cumDist   []float64 // cumDist[i] = flight distance from takeoff to points[i]
	segments  []taxiSegment
	splineIDs []uint32
	segIdx    int
	hopIdx    int
}

// taxiDiscountedLeg mirrors the per-leg ceil in
// FlightPathMovementGenerator::LoadPath (FlightPathMovementGenerator.cpp:224).
func taxiDiscountedLeg(rawPrice uint64, discount float64) uint32 {
	return uint32(math.Ceil(float64(rawPrice) * discount))
}

// formatTaxiPath mirrors PlayerTaxi::SaveTaxiDestinationsToString
// (PlayerTaxi.cpp:151-163): "faction n0 n1 ... " with the trailing space.
func formatTaxiPath(faction uint32, nodes []uint32) string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(uint64(faction), 10))
	for _, n := range nodes {
		sb.WriteByte(' ')
		sb.WriteString(strconv.FormatUint(uint64(n), 10))
	}
	sb.WriteByte(' ')
	return sb.String()
}

// mergeTaxiHopPoints concatenates each hop's TaxiPathNode vertices into one
// spline — the Go twin of FlightPathMovementGenerator::LoadPath's merged
// _path (FlightPathMovementGenerator.cpp:196-222). LoadPath shortens the
// merged path with a 40-yard (X/Y, same-map) proximity rule; the observable
// effect at hop joints is that the shared flightmaster node collapses to a
// single vertex, which the dedup below reproduces. It also returns each
// hop's end vertex index (the _pointsForPathSwitch PathIndex) and the
// cumulative flight distance at every vertex for arrival timing.
func mergeTaxiHopPoints(store *wotlk.Store, pathIDs []uint32, startX, startY, startZ float32) ([]wotlk.TaxiSplinePoint, []int, []float64, bool) {
	var points []wotlk.TaxiSplinePoint
	endPts := make([]int, 0, len(pathIDs))
	for i, pathID := range pathIDs {
		pts, err := store.TaxiPathPoints(pathID)
		if err != nil || len(pts) == 0 {
			return nil, nil, nil, false
		}
		start := 0
		if i > 0 && len(points) > 0 {
			tail := points[len(points)-1]
			for start < len(pts)-1 {
				dx := pts[start].X - tail.X
				dy := pts[start].Y - tail.Y
				if pts[start].MapID != tail.MapID || dx*dx+dy*dy > 40*40 {
					break
				}
				start++
			}
		}
		points = append(points, pts[start:]...)
		endPts = append(endPts, len(points)-1)
	}
	cumDist := make([]float64, len(points))
	prevX, prevY, prevZ := startX, startY, startZ
	for i, p := range points {
		dx := float64(p.X - prevX)
		dy := float64(p.Y - prevY)
		dz := float64(p.Z - prevZ)
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if i == 0 {
			cumDist[0] = d
		} else {
			cumDist[i] = cumDist[i-1] + d
		}
		prevX, prevY, prevZ = p.X, p.Y, p.Z
	}
	return points, endPts, cumDist, true
}

// validTaxiFlight reports whether f is still the session's live flight.
func (s *session) validTaxiFlight(f *taxiFlight) bool {
	s.taxiMu.Lock()
	defer s.taxiMu.Unlock()
	return f != nil && s.taxiFlight == f && s.inFlight && s.player != nil
}

// currentTaxiFlight returns the live flight, if any.
func (s *session) currentTaxiFlight() *taxiFlight {
	s.taxiMu.Lock()
	defer s.taxiMu.Unlock()
	if !s.inFlight {
		return nil
	}
	return s.taxiFlight
}

// chargeTaxiLegAmount deducts one hop's price: the C++ ModifyMoney(-cost)
// (Player.cpp:21668, FlightPathMovementGenerator.cpp:121-122), which clamps
// at zero instead of failing — the departure gate already ran.
func (s *session) chargeTaxiLegAmount(ctx context.Context, cost uint32) {
	if cost == 0 || s == nil || s.player == nil {
		return
	}
	if s.player.Money < cost {
		s.player.Money = 0
	} else {
		s.player.Money -= cost
	}
	s.questMoneyChanged(ctx)
	s.updateAchievementCriteria(criteriaTypeGoldSpentTravel, 0, cost)
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	}
	s.sendPlayerMoneyUpdate()
}

// beginTaxiFlight starts (or resumes) a multi-hop taxi flight: the Go twin of
// Player::ActivateTaxiPathTo's DO FLIGHT arm plus
// FlightPathMovementGenerator::LoadPath/DoReset. nodes[0] is the current
// source; prices[i] is the raw hop price for nodes[i]->nodes[i+1]. resumePt
// trims that many merged vertices (Player::ContinueTaxiFlight's startNode
// search); chargeFirstHop mirrors the departure firstcost deduction (false
// on the ContinueTaxiFlight path, whose current hop was already charged).
func (s *session) beginTaxiFlight(ctx context.Context, nodes []uint32, pathIDs []uint32, prices []uint64, discount float64, faction, mount uint32, resumePt int, chargeFirstHop bool) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || len(nodes) < 2 || len(pathIDs) != len(nodes)-1 || len(prices) != len(pathIDs) {
		return false
	}
	points, endPts, cumDist, ok := mergeTaxiHopPoints(s.server.Data, pathIDs, s.player.X, s.player.Y, s.player.Z)
	if !ok || len(points) == 0 {
		return false
	}
	if resumePt > 0 {
		if resumePt >= len(points) || resumePt > endPts[0] {
			return false
		}
		points = points[resumePt:]
		for i := range endPts {
			endPts[i] -= resumePt
		}
		rebased := make([]float64, len(cumDist)-resumePt)
		for i := range rebased {
			rebased[i] = cumDist[resumePt+i] - cumDist[resumePt]
		}
		cumDist = rebased
	}
	var segments []taxiSegment
	segStart := 0
	for i := 1; i < len(points); i++ {
		if points[i].MapID != points[i-1].MapID {
			segments = append(segments, taxiSegment{segStart, i})
			segStart = i
		}
	}
	segments = append(segments, taxiSegment{segStart, len(points)})
	f := &taxiFlight{
		faction:   faction,
		mount:     mount,
		discount:  discount,
		nodes:     append([]uint32(nil), nodes...),
		points:    points,
		cumDist:   cumDist,
		segments:  segments,
		splineIDs: make([]uint32, len(segments)),
	}
	f.hops = make([]taxiHop, len(pathIDs))
	for i := range pathIDs {
		f.hops[i] = taxiHop{pathID: pathIDs[i], rawPrice: prices[i], endPt: endPts[i], endDist: cumDist[endPts[i]]}
	}
	s.taxiMu.Lock()
	s.taxiFlight = f
	s.inFlight = true
	s.taxiMu.Unlock()
	if chargeFirstHop {
		// Player::ActivateTaxiPathTo (Player.cpp:21668): the first leg is
		// deducted at departure; later legs on hop arrival.
		s.chargeTaxiLegAmount(ctx, taxiDiscountedLeg(prices[0], discount))
	}
	return s.sendTaxiSegment(f, 0)
}

// sendTaxiSegment flies one map-uniform segment: the SMSG_MONSTER_MOVE for
// the C++ DoReset spline slice (FlightPathMovementGenerator.cpp:93-114).
// It pins the server position to the segment end and schedules the hop
// arrival timers inside the segment plus the segment-end timer.
func (s *session) sendTaxiSegment(f *taxiFlight, segIdx int) bool {
	s.taxiMu.Lock()
	live := s.taxiFlight == f && s.inFlight && s.player != nil && f.segIdx == segIdx && segIdx < len(f.segments)
	hopIdx := f.hopIdx
	s.taxiMu.Unlock()
	if !live {
		return false
	}
	seg := f.segments[segIdx]
	pts := f.points[seg.startPt:seg.endPt]
	if len(pts) == 0 {
		return false
	}
	if f.mount != 0 {
		s.player.MountDisplayID = f.mount
		s.sendPlayerMountUpdate()
	}
	splineID := atomic.AddUint32(&taxiSplineIDSource, 1)
	s.taxiMu.Lock()
	f.splineIDs[segIdx] = splineID
	s.taxiMu.Unlock()
	s.sendTaxiMonsterMove(pts, splineID)
	last := pts[len(pts)-1]
	s.player.X, s.player.Y, s.player.Z = last.X, last.Y, last.Z
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(),
			"UPDATE characters SET map = ?, position_x = ?, position_y = ?, position_z = ? WHERE guid = ?",
			s.player.Map, last.X, last.Y, last.Z, s.playerGUID)
	}
	segBase := 0.0
	if seg.startPt > 0 {
		segBase = f.cumDist[seg.startPt]
	}
	segDist := f.cumDist[seg.endPt-1] - segBase
	if segDist <= 0 {
		segDist = 1
	}
	segDuration := time.Duration(segDist/28.0*1000) * time.Millisecond
	if segDuration < 500*time.Millisecond {
		segDuration = 500 * time.Millisecond
	}
	for h := hopIdx; h < len(f.hops); h++ {
		endPt := f.hops[h].endPt
		if endPt < seg.startPt || endPt >= seg.endPt-1 {
			continue
		}
		delay := time.Duration((f.cumDist[endPt] - segBase) / segDist * float64(segDuration))
		hop, sg := h, segIdx
		time.AfterFunc(delay, func() { s.onTaxiHopArrival(f, sg, hop) })
	}
	sg := segIdx
	time.AfterFunc(segDuration, func() { s.onTaxiSegmentEnd(f, sg) })
	return true
}

// sendTaxiMonsterMove broadcasts one flight spline: SMSG_MONSTER_MOVE with
// Flying | Catmullrom (MoveSplineFlag), the C++ SendDoFlight/MoveTaxiFlight
// spline launch (TaxiHandler.cpp:115-125, MotionMaster.cpp:983-1005).
func (s *session) sendTaxiMonsterMove(pts []wotlk.TaxiSplinePoint, splineID uint32) {
	packet := protocol.NewBuffer(32 + len(pts)*12)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU8(0) // MOVEMENTFLAG2_UNK7
	packet.WriteF32(s.player.X)
	packet.WriteF32(s.player.Y)
	packet.WriteF32(s.player.Z)
	packet.WriteU32(splineID)
	packet.WriteU8(0)           // MonsterMoveNormal
	packet.WriteU32(0x00042000) // Flying | Catmullrom
	total := float64(0)
	prevX, prevY, prevZ := s.player.X, s.player.Y, s.player.Z
	for _, point := range pts {
		total += math.Sqrt(float64((point.X-prevX)*(point.X-prevX) + (point.Y-prevY)*(point.Y-prevY) + (point.Z-prevZ)*(point.Z-prevZ)))
		prevX, prevY, prevZ = point.X, point.Y, point.Z
	}
	// Intercontinental flight speed baseline; the client scales the spline
	// by the provided duration.
	speed := 28.0
	duration := uint32((total / speed) * 1000)
	if duration < 500 {
		duration = 500
	}
	packet.WriteU32(duration)
	packet.WriteU32(uint32(len(pts)))
	for _, point := range pts {
		packet.WriteF32(point.X)
		packet.WriteF32(point.Y)
		packet.WriteF32(point.Z)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_MONSTER_MOVE), packet.Bytes(), true)
}

// onTaxiHopArrival fires when the flight reaches a hop's last vertex: the Go
// twin of the DoUpdate path-switch pop (FlightPathMovementGenerator.cpp:117-128)
// — NextTaxiDestination advances the deque and the next leg's price is deducted.
func (s *session) onTaxiHopArrival(f *taxiFlight, segIdx, hop int) {
	s.taxiMu.Lock()
	if s.taxiFlight != f || !s.inFlight || s.player == nil || f.segIdx != segIdx || f.hopIdx != hop || hop+1 >= len(f.hops) {
		s.taxiMu.Unlock()
		return
	}
	next := hop + 1
	f.hopIdx = next
	if len(f.nodes) > 1 {
		f.nodes = f.nodes[1:]
	}
	s.player.TaxiPath = formatTaxiPath(f.faction, f.nodes)
	cost := taxiDiscountedLeg(f.hops[next].rawPrice, f.discount)
	s.taxiMu.Unlock()
	s.chargeTaxiLegAmount(context.Background(), cost)
	s.debug("taxi hop arrival", "account", s.accountName, "hop", next, "cost", cost)
}

// popTaxiHop completes the current hop when the segment-end handler catches
// a boundary the arrival timer missed: same NextTaxiDestination + charge as
// onTaxiHopArrival. It never pops the final hop — the final leg was charged
// at its start boundary and nothing follows it.
func (s *session) popTaxiHop(f *taxiFlight) bool {
	s.taxiMu.Lock()
	if s.taxiFlight != f || !s.inFlight || s.player == nil || f.hopIdx >= len(f.hops)-1 {
		s.taxiMu.Unlock()
		return false
	}
	next := f.hopIdx + 1
	f.hopIdx = next
	if len(f.nodes) > 1 {
		f.nodes = f.nodes[1:]
	}
	s.player.TaxiPath = formatTaxiPath(f.faction, f.nodes)
	cost := taxiDiscountedLeg(f.hops[next].rawPrice, f.discount)
	s.taxiMu.Unlock()
	s.chargeTaxiLegAmount(context.Background(), cost)
	return true
}

// onTaxiSegmentEnd runs when a segment's spline completes — driven by the
// segment timer or the client's CMSG_MOVE_SPLINE_DONE, whichever wins the
// race (the loser sees the claimed segIdx and drops out). It completes hops
// ending at the boundary, then either far-teleports onto the next segment
// (HandleMoveSplineDoneOpcode's far-teleport arm, TaxiHandler.cpp:214-233)
// or runs the final arrival.
func (s *session) onTaxiSegmentEnd(f *taxiFlight, segIdx int) {
	s.taxiMu.Lock()
	if s.taxiFlight != f || !s.inFlight || f.segIdx != segIdx || segIdx >= len(f.segments) {
		s.taxiMu.Unlock()
		return
	}
	f.segIdx++ // claim the transition; concurrent losers see the mismatch
	seg := f.segments[segIdx]
	s.taxiMu.Unlock()
	for {
		s.taxiMu.Lock()
		more := f.hopIdx < len(f.hops)-1 && f.hops[f.hopIdx].endPt < seg.endPt
		s.taxiMu.Unlock()
		if !more || !s.popTaxiHop(f) {
			break
		}
	}
	if segIdx == len(f.segments)-1 {
		s.finishTaxiArrival(f)
		return
	}
	// The flight may have been interrupted while the hops were settling;
	// never teleport a dead flight.
	s.taxiMu.Lock()
	alive := s.taxiFlight == f && s.inFlight
	s.taxiMu.Unlock()
	if !alive {
		return
	}
	// Far-teleport arm: the next segment is on another map. C++ teleports to
	// the first node past the map boundary (SetCurrentNodeAfterTeleport +
	// SkipCurrentNode, then TeleportTo to the destination continent); the
	// new spline is what carries the client onward.
	nextSeg := f.segments[segIdx+1]
	pt := f.points[nextSeg.startPt]
	if s.player != nil {
		s.player.Map = uint32(pt.MapID)
		s.player.InstanceID = 0
		s.player.X, s.player.Y, s.player.Z = pt.X, pt.Y, pt.Z
		s.updateZoneAndArea(context.Background(), true)
	}
	s.debug("taxi far teleport", "account", s.accountName, "map", pt.MapID, "x", pt.X, "y", pt.Y, "z", pt.Z)
	s.sendTaxiSegment(f, segIdx+1)
}

// finishTaxiArrival runs the final-arrival arms:
// FlightPathMovementGenerator::DoFinalize (FlightPathMovementGenerator.cpp:154-178:
// dismount, benchmark clear) plus the HandleMoveSplineDoneOpcode final arm
// (TaxiHandler.cpp:236-247: fall info reset, hostile-zone Honorless Target).
func (s *session) finishTaxiArrival(f *taxiFlight) {
	s.taxiMu.Lock()
	if s.taxiFlight != f {
		s.taxiMu.Unlock()
		return
	}
	s.taxiFlight = nil
	s.inFlight = false
	s.taxiMu.Unlock()
	if s.player == nil {
		return
	}
	s.player.TaxiPath = ""
	// HandleMoveSplineDoneOpcode (TaxiHandler.cpp:241-247): on taxi arrival
	// the fall info resets and a hostile-zone landing earns Honorless Target
	// (2479, triggered). The zone state refreshes here because Go pins the
	// arrival position at flight start and the client sends no movement
	// mid-flight, so s.pvpHostile would otherwise be stale from the
	// departure zone.
	s.updateZoneAndArea(context.Background(), true)
	s.lastFallZ, s.lastFallTime = s.player.Z, 0
	if s.pvpHostile {
		s.castSpellDirect(context.Background(), 2479, s.playerGUID)
	}
	if s.currentPlayer() != nil {
		s.player.MountDisplayID = 0
		s.sendPlayerMountUpdate()
	}
	s.clearTaxiBenchmarkMode()
}

// isInFlight mirrors Unit::IsInFlight (UNIT_STATE_IN_FLIGHT).
func (s *session) isInFlight() bool {
	return s.inFlight
}

// clearTaxiBenchmarkMode mirrors FlightPathMovementGenerator::DoFinalize
// (FlightPathMovementGenerator.cpp:176), which clears
// PLAYER_FLAGS_TAXI_BENCHMARK whenever the flight movement generator
// finalizes — natural arrival or interruption.
func (s *session) clearTaxiBenchmarkMode() {
	if s == nil || s.player == nil {
		return
	}
	if s.player.PlayerFlags&playerFlagTaxiBenchmark != 0 {
		s.player.PlayerFlags &^= playerFlagTaxiBenchmark
		s.sendPlayerUpdate()
	}
}

// finishTaxiFlight mirrors Player::FinishTaxiFlight: it interrupts the
// flight (teleports, BG entry, logout, ...). Pending hop/segment timers die
// on the flight-pointer check.
func (s *session) finishTaxiFlight() {
	s.taxiMu.Lock()
	wasInFlight := s.inFlight
	s.taxiFlight = nil
	s.inFlight = false
	s.taxiMu.Unlock()
	if !wasInFlight || s.player == nil {
		return
	}
	s.player.TaxiPath = ""
	if s.currentPlayer() != nil && s.player.MountDisplayID != 0 {
		s.player.MountDisplayID = 0
		s.sendPlayerMountUpdate()
	}
	s.clearTaxiBenchmarkMode()
}

// initTaxiNodesForLevel mirrors PlayerTaxi::InitTaxiNodesForLevel for the
// race/team starting nodes available without discovering them first.
func (s *session) initTaxiNodesForLevel() {
	if s.player == nil {
		return
	}
	if s.player.Class == 6 && s.server != nil && s.server.Data != nil {
		if nodes, err := s.server.Data.OldContinentsTaxiNodes(); err == nil {
			for _, node := range nodes {
				s.setTaxiMaskNode(node)
			}
		}
	}
	// Race specific initial known nodes: capital and taxi hub.
	switch s.player.Race {
	case 1: // Human
		s.setTaxiMaskNode(2)
	case 3: // Dwarf
		s.setTaxiMaskNode(6)
	case 4: // Night Elf
		s.setTaxiMaskNode(26)
		s.setTaxiMaskNode(27)
	case 7: // Gnome
		s.setTaxiMaskNode(6)
	case 11: // Draenei
		s.setTaxiMaskNode(94)
	case 2: // Orc
		s.setTaxiMaskNode(23)
	case 5: // Undead
		s.setTaxiMaskNode(11)
	case 6: // Tauren
		s.setTaxiMaskNode(22)
	case 8: // Troll
		s.setTaxiMaskNode(23)
	case 10: // Blood Elf
		s.setTaxiMaskNode(82)
	}
	// New continent starting node (Dalaran area by team).
	if s.playerAlliance() {
		s.setTaxiMaskNode(100)
	} else {
		s.setTaxiMaskNode(99)
	}
	if s.player.Level >= 68 {
		s.setTaxiMaskNode(213)
	}
}

// objectFloat32Field reads a float object field set by luaCreature.
func objectFloat32Field(object *scripting.Object, name string) (float32, bool) {
	value, ok := object.Fields[name]
	if !ok {
		return 0, false
	}
	switch value := value.(type) {
	case float32:
		return value, true
	case float64:
		return float32(value), true
	case uint32:
		return float32(value), true
	case int64:
		return float32(value), true
	default:
		return 0, false
	}
}

// handleEnableTaxi processes CMSG_ENABLETAXI (0x493).
// handleEnableTaxi is an alias for CMSG_TAXIQUERYAVAILABLENODES: C++
// (Opcodes.cpp:1302) binds CMSG_ENABLETAXI to
// WorldSession::HandleTaxiQueryAvailableNodes, which learns an unknown
// node and opens the taxi window — the client uses both opcodes for the
// same interaction, so both route to handleTaxiQueryAvailableNodes.
func (s *session) handleEnableTaxi(ctx context.Context, payload []byte) bool {
	return s.handleTaxiQueryAvailableNodes(ctx, payload)
}

// handleSetTaxiBenchmarkMode processes CMSG_SET_TAXI_BENCHMARK_MODE (0x389).
// Reference: WorldSession::HandleSetTaxiBenchmarkOpcode (MiscHandler.cpp:1403).
func (s *session) handleSetTaxiBenchmarkMode(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	mode := payload[0]
	if mode != 0 {
		s.player.PlayerFlags |= playerFlagTaxiBenchmark
	} else {
		s.player.PlayerFlags &^= playerFlagTaxiBenchmark
	}
	s.sendPlayerUpdate()
	return true
}

// handleEffectSendTaxi processes SPELL_EFFECT_SEND_TAXI (123).
// Mirrors TrinityCore Spell::EffectSendTaxi (SpellEffects.cpp:4656-4664),
// which calls Player::ActivateTaxiPathTo(taxi_path_id, spellid) with the
// path id from the effect's MiscValue (Player.cpp:21689). The vector
// overload (Player.cpp:21510) runs with npc == nullptr (spell case): the
// logout/combat/stun/root gate replies ERR_TAXIPLAYERBUSY, mounted auras
// are stripped, the source node must exist, trade is cancelled, the
// single hop resolves through GetTaxiPath, the wide mount lookup applies
// (npc == nullptr allows it), insufficient money replies
// ERR_TAXINOTENOUGHMONEY, and the flight starts with the first hop's cost
// charged. The logout/stun/root/combat busy arms are the shared vector-arm
// gates (Player.cpp:21517-21523), same as the handler path; the
// generic/autorepeat/channeled interrupt arm is vacuous — the effect
// fires at HIT_TARGET after its own cast completed.
func (s *session) handleEffectSendTaxi(ctx context.Context, eff wotlk.SpellEffect) {
	if s == nil || !s.playerLoaded || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	reply := func(code uint32) {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(code)
		_ = s.write(uint16(protocol.OpcodeSMSG_ACTIVATETAXIREPLY), packet.Bytes(), true)
	}
	if eff.MiscValue <= 0 {
		return
	}
	pathID := uint32(eff.MiscValue)
	from, to, price, found, err := s.server.Data.TaxiPathEntry(pathID)
	if err != nil || !found {
		return
	}
	// Player.cpp:21517: no flight while logging out, in combat, stunned or
	// rooted — the busy gate runs before the npc check, so it applies to
	// the spell case too (path-id overload delegates to the vector arm
	// with npc == nullptr).
	if !s.logoutAt.IsZero() || s.isInCombat() || s.player.UnitFlags&unitFlagStunned != 0 || s.rooted {
		reply(taxiErrPlayerBusy)
		return
	}
	// Player.cpp:21551 (spell case): strip the SPELL_AURA_MOUNTED auras —
	// the aura remove leg runs the Unit::Dismount mirror (display/flag
	// clear, SMSG_DISMOUNT, pet resummon), not just the display field.
	s.removeAurasByType(spellAuraMounted)
	// Player.cpp:21562: the source node must exist.
	if !s.server.Data.TaxiNodeExists(from) {
		reply(taxiErrNoSuchPath)
		return
	}
	// Player.cpp:21580: the player leaves any vehicle before the flight
	// starts (ExitVehicle in ActivateTaxiPathTo is unconditional on the
	// npc==nullptr spell path too) — exitVehicle is a no-op when not
	// mounted on a vehicle.
	s.exitVehicle(ctx)
	s.cancelTrade(true)
	// Player.cpp:21588: resolve the hop through GetTaxiPath. A missing
	// sourcepath answers ERR_TAXIUNSPECIFIEDSERVERERROR (Player.cpp:21613);
	// the mount-0 allowance is spell-case-only and checked implicitly by
	// skipping the mount gate below.
	resolvedPath, _, hopFound, hopErr := s.server.Data.TaxiPathLinks(from, to)
	if hopErr != nil || !hopFound || resolvedPath == 0 {
		reply(taxiErrUnspecified)
		return
	}
	// Player.cpp:21611: wide mount lookup for the spell case (npc == nil).
	mount := s.taxiMountDisplayID(ctx, from, true)
	if s.player.Money < price {
		reply(taxiErrNotEnoughMoney)
		return
	}
	s.updateAchievementCriteria(criteriaTypeFlightPathsTaken, 0, 1)
	// Player.cpp:21645: the spell case records no flight-master faction (0).
	// The single hop's raw price is charged at departure (the firstcost arm);
	// the spell path never discounts (npc == nullptr, Player.cpp:21644).
	previousTaxiPath := s.player.TaxiPath
	s.player.TaxiPath = formatTaxiPath(0, []uint32{from, to})
	if !s.beginTaxiFlight(ctx, []uint32{from, to}, []uint32{pathID}, []uint64{uint64(price)}, 1.0, 0, mount, 0, true) {
		s.player.TaxiPath = previousTaxiPath
		return
	}
	reply(taxiErrOK)
}

// handleEffectDiscoverTaxi processes SPELL_EFFECT_DISCOVER_TAXI (154).
// Mirrors TrinityCore Spell::EffectDiscoverTaxi (SpellEffects.cpp:5390-5398):
// when the node id from the effect's MiscValue names a known
// TaxiNodes.dbc row, WorldSession::SendDiscoverNewTaxiNode sets the mask
// bit and sends SMSG_NEW_TAXI_PATH (only the path packet — unlike the
// flight-master greeting path, no SMSG_TAXINODE_STATUS).
func (s *session) handleEffectDiscoverTaxi(ctx context.Context, eff wotlk.SpellEffect) {
	if s == nil || !s.playerLoaded || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	if eff.MiscValue <= 0 {
		return
	}
	nodeID := uint32(eff.MiscValue)
	if !s.server.Data.TaxiNodeExists(nodeID) {
		return
	}
	if s.isTaxiMaskNodeKnown(nodeID) {
		return
	}
	if s.setTaxiMaskNode(nodeID) {
		s.saveTaxiMask(ctx)
		_ = s.write(uint16(protocol.OpcodeSMSG_NEW_TAXI_PATH), nil, true)
	}
}
