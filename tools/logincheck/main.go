package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/world"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

type loginStage struct {
	Name  string
	Match func(uint32) bool
}

func main() {
	tracePath := flag.String("trace", "", "recorded protocol trace JSONL")
	selfCheck := flag.Bool("self-check", false, "validate the login loading-order regression guard")
	replayWork := flag.String("replay-work", "", "isolated work directory containing auth.db, characters.db, and world.db; runs core login with Eluna disabled")
	replayGUID := flag.Uint64("replay-guid", 0, "character GUID for an in-process login replay; 0 selects the first real character")
	replayPetCooldownSpell := flag.Uint("replay-pet-cooldown-spell", 0, "after login, assert a saved pet category cooldown rejects this spell")
	replayPetPowerSpell := flag.Uint("replay-pet-power-spell", 0, "after login, verify a known pet spell spends and reports its DBC power cost")
	replayPetXPAward := flag.Uint("replay-pet-xp", 0, "after login, apply a hunter-pet XP award and verify fields and persistence")
	replayTrace := flag.String("trace-out", "", "optional JSONL path for the in-process login trace")
	flag.Parse()
	if *selfCheck {
		if err := runSelfCheck(); err != nil {
			fail(err.Error())
		}
		fmt.Println("login loading-order self-check passed")
		return
	}
	if *replayWork != "" {
		if (*replayPetCooldownSpell != 0 && *replayPetPowerSpell != 0) || (*replayPetCooldownSpell != 0 && *replayPetXPAward != 0) || (*replayPetPowerSpell != 0 && *replayPetXPAward != 0) {
			fail("choose only one pet replay scenario")
		}
		if err := runRealCharacterLoginReplay(*replayWork, *replayGUID, *replayTrace, uint32(*replayPetCooldownSpell), uint32(*replayPetPowerSpell), uint32(*replayPetXPAward)); err != nil {
			fail(err.Error())
		}
		return
	}
	if *tracePath == "" {
		fail("-trace is required")
	}
	file, err := os.Open(*tracePath)
	if err != nil {
		fail(err.Error())
	}
	trace, err := protocoltrace.Load(file)
	file.Close()
	if err != nil {
		fail(err.Error())
	}
	loginOpcode := uint32(protocol.OpcodeCMSG_PLAYER_LOGIN)
	checked := 0
	for index, event := range trace.Events {
		if event.Direction != protocoltrace.ClientToServer || event.Opcode != loginOpcode {
			continue
		}
		if err := checkLogin(trace, index); err != nil {
			fail(fmt.Sprintf("login %d: %v", checked+1, err))
		}
		checked++
	}
	if checked == 0 {
		fail("trace contains no CMSG_PLAYER_LOGIN event")
	}
	fmt.Printf("login traces checked=%d order=create-update-gate-passed\n", checked)
}

func runSelfCheck() error {
	if world.PlayerCreateUpdateFlags(false, false) != 0x0060 || world.PlayerCreateUpdateFlags(true, false) != 0x0061 || world.PlayerCreateUpdateFlags(false, true) != 0x0064 {
		return fmt.Errorf("player create update flags do not match victim/self source flags")
	}
	if err := checkMapEntryEvent(); err != nil {
		return fmt.Errorf("map entry hook check failed: %w", err)
	}
	if speed := world.ResolveMovementSpeed(1, -90, 60); speed != 0.6 {
		return fmt.Errorf("movement slow/minimum-speed ordering produced %f", speed)
	}
	if multiplier := world.ResolveAuraPercentMultiplier([]int32{20, 10}); math.Abs(float64(multiplier-1.32)) > 0.001 {
		return fmt.Errorf("stacked aura multiplier produced %f", multiplier)
	}
	if multiplier := world.ResolveCastSpeedMultiplier([]int32{20, -20}); math.Abs(float64(multiplier-1)) > 0.001 {
		return fmt.Errorf("cast-time aura multiplier produced %f", multiplier)
	}
	if err := checkGameDataPathResolution(); err != nil {
		return fmt.Errorf("game data path resolution check failed: %w", err)
	}
	if err := checkTransportTrajectory(); err != nil {
		return fmt.Errorf("transport trajectory check failed: %w", err)
	}
	baseRuneTypes := [6]uint8{0, 0, 1, 1, 2, 2}
	runeTypes := world.ResolveDeathKnightRuneTypes(baseRuneTypes, []wotlk.Spell{{Attributes: 0x40, Effects: [3]wotlk.SpellEffect{{Effect: 6, Aura: 249, MiscValue: 0, MiscValueB: 3}, {Effect: 6, Aura: 249, BasePoints: 1, MiscValue: 1, MiscValueB: 3}}}, {Effects: [3]wotlk.SpellEffect{{Effect: 6, Aura: 249, MiscValue: 0, MiscValueB: 3}}}})
	if runeTypes != [6]uint8{3, 0, 3, 3, 2, 2} {
		return fmt.Errorf("passive rune conversions produced rune types %v", runeTypes)
	}
	login := uint32(protocol.OpcodeCMSG_PLAYER_LOGIN)
	verify := uint32(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD)
	criteria := uint32(protocol.OpcodeSMSG_CRITERIA_UPDATE)
	bad := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: criteria}}}
	if err := rejectPreVerifyAchievementPackets(bad, 0); err == nil {
		return fmt.Errorf("pre-verify achievement packet was not rejected")
	}
	good := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: verify}}}
	if err := rejectPreVerifyAchievementPackets(good, 0); err != nil {
		return fmt.Errorf("valid verify-world ordering was rejected: %w", err)
	}
	goodAchievement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: criteria}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_ACHIEVEMENT_EARNED)}}}
	if err := rejectPreVerifyAchievementPackets(goodAchievement, 0); err != nil {
		return fmt.Errorf("post-verify achievement packet was rejected: %w", err)
	}
	loginGUID := uint64(1)
	loginPayload := protocol.NewBuffer(8)
	loginPayload.WriteU64(loginGUID)
	loginEvent := protocoltrace.Event{Direction: protocoltrace.ClientToServer, Opcode: login, Payload: base64.StdEncoding.EncodeToString(loginPayload.Bytes())}
	playerCreateEvent, err := loginCreateFixture(false, 0)
	if err != nil {
		return fmt.Errorf("login create fixture build failed: %w", err)
	}
	loginOrderStages := []loginStage{{"difficulty", exact(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY)}, {"verify", exact(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD)}, {"contact", exact(protocol.OpcodeSMSG_CONTACT_LIST)}, {"player create update", exact(protocol.OpcodeSMSG_UPDATE_OBJECT)}, {"world states", exact(protocol.OpcodeSMSG_INIT_WORLD_STATES)}}
	validLoginOrder := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY)}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_CONTACT_LIST)}, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}}}
	if _, err := findOrderedLoginStages(validLoginOrder, 0, loginOrderStages); err != nil {
		return fmt.Errorf("valid login stage order was rejected: %w", err)
	}
	badEarlyLoginOrder := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_CONTACT_LIST)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY)}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_CONTACT_LIST)}, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}}}
	if _, err := findOrderedLoginStages(badEarlyLoginOrder, 0, loginOrderStages); err == nil {
		return fmt.Errorf("out-of-order pre-map login packet was not rejected")
	}
	badEarlyWorldStates := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY)}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_CONTACT_LIST)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}}}
	if _, err := findOrderedLoginStages(badEarlyWorldStates, 0, loginOrderStages); err == nil {
		return fmt.Errorf("post-map world state sent before player create was not rejected")
	}
	validMovement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_FEATHER_FALL)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_HOVER)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_FORCE_MOVE_ROOT)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MULTIPLE_MOVES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL)}}}
	if err := checkLoginMovementOrder(validMovement, 0); err != nil {
		return fmt.Errorf("valid movement ordering was rejected: %w", err)
	}
	guildEventTrace := func(eventType uint8) protocoltrace.Event {
		return protocoltrace.Event{Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_GUILD_EVENT), Payload: base64.StdEncoding.EncodeToString([]byte{eventType})}
	}
	guildOrder := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOTD)}, guildEventTrace(2), {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_GUILD_BANK_LIST)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_GUILD_ROSTER)}, guildEventTrace(12), {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_LEARNED_DANCE_MOVES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)}}}
	if err := checkPreMapGuildLoginOrder(guildOrder, 0, 7); err != nil {
		return fmt.Errorf("valid pre-map guild ordering was rejected: %w", err)
	}
	runeOrder := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_SET_FORCED_REACTIONS)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_RESYNC_RUNES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)}}}
	if err := checkOptionalPreMapRuneOrder(runeOrder, 0, 3); err != nil {
		return fmt.Errorf("valid pre-map rune ordering was rejected: %w", err)
	}
	runtimeRuneOrder := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_SET_FORCED_REACTIONS)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_RESYNC_RUNES)}}}
	if err := checkOptionalPreMapRuneOrder(runtimeRuneOrder, 0, 2); err != nil {
		return fmt.Errorf("runtime rune resync was rejected: %w", err)
	}
	badRuneOrder := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_RESYNC_RUNES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_SET_FORCED_REACTIONS)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)}}}
	if err := checkOptionalPreMapRuneOrder(badRuneOrder, 0, 3); err == nil {
		return fmt.Errorf("pre-map rune resync before forced reactions was not rejected")
	}
	power := uint32(777)
	target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: loginGUID}
	spellGoTrace := func(spellID uint32) protocoltrace.Event {
		payload := protocol.BuildSpellGoWithPower(loginGUID, loginGUID, 0, spellID, 0x40901, 123, []uint64{loginGUID}, nil, target, &power)
		return protocoltrace.Event{Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_SPELL_GO), Payload: base64.StdEncoding.EncodeToString(payload)}
	}
	validDurationOrder := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, spellGoTrace(836), {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_ITEM_ENCHANT_TIME_UPDATE)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_ITEM_TIME_UPDATE)}}}
	if err := checkPostMapLoginOrder(validDurationOrder, 0, 1); err != nil {
		return fmt.Errorf("valid post-map duration ordering was rejected: %w", err)
	}
	badDurationOrder := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, spellGoTrace(836), {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_ITEM_TIME_UPDATE)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_ITEM_ENCHANT_TIME_UPDATE)}}}
	if err := checkPostMapLoginOrder(badDurationOrder, 0, 1); err == nil {
		return fmt.Errorf("reversed post-map duration ordering was not rejected")
	}
	badGuildOrder := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOTD)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_GUILD_ROSTER)}, guildEventTrace(2), {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_LEARNED_DANCE_MOVES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)}}}
	if err := checkPreMapGuildLoginOrder(badGuildOrder, 0, 5); err == nil {
		return fmt.Errorf("out-of-order pre-map guild packets were not rejected")
	}
	for _, test := range []struct {
		current, want   uint16
		inventory       uint32
		required, count uint32
		added           bool
	}{{2, 3, 0, 3, 2, true}, {2, 1, 0, 3, 1, false}, {3, 1, 2, 3, 1, false}, {3, 0, 0, 3, 1, false}, {3, 3, 0, 3, 1, true}, {10, 9, 10, 10, 1, false}} {
		if actual := world.QuestItemCountAfterDelta(test.current, test.inventory, test.required, test.count, test.added); actual != test.want {
			return fmt.Errorf("quest item count transition was %d, want %d", actual, test.want)
		}
	}
	if err := checkCreateMovementParser(); err != nil {
		return err
	}
	missingLoginEffect := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}}}
	if err := checkPostMapLoginOrder(missingLoginEffect, 0, 1); err == nil {
		return fmt.Errorf("missing source-required post-map spell 836 was not rejected")
	}
	wrongLoginEffectTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: loginGUID + 1}
	wrongLoginEffectPayload := protocol.BuildSpellGoWithPower(loginGUID+1, loginGUID+1, 0, 836, 0x40901, 123, []uint64{loginGUID + 1}, nil, wrongLoginEffectTarget, &power)
	wrongLoginEffect := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_SPELL_GO), Payload: base64.StdEncoding.EncodeToString(wrongLoginEffectPayload)}}}
	if err := checkPostMapLoginOrder(wrongLoginEffect, 0, 1); err == nil {
		return fmt.Errorf("login spell 836 targeting another unit was not rejected")
	}
	wrongLoginEffectFlagsPayload := protocol.BuildSpellGoWithPower(loginGUID, loginGUID, 0, 836, 0x901, 123, []uint64{loginGUID}, nil, target, &power)
	wrongLoginEffectFlags := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_SPELL_GO), Payload: base64.StdEncoding.EncodeToString(wrongLoginEffectFlagsPayload)}}}
	if err := checkPostMapLoginOrder(wrongLoginEffectFlags, 0, 1); err == nil {
		return fmt.Errorf("login spell 836 without its source no-GCD flag was not rejected")
	}
	preMapLoginEffect := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, spellGoTrace(836), playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, spellGoTrace(836)}}
	if err := checkPostMapLoginOrder(preMapLoginEffect, 0, 2); err == nil {
		return fmt.Errorf("pre-map spell 836 was not rejected")
	}
	postMapTrace := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, playerCreateEvent, spellGoTrace(57940), {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, spellGoTrace(836), {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE)}}}
	if err := checkPostMapLoginOrder(postMapTrace, 0, 1); err != nil {
		return fmt.Errorf("zone spell before world states was rejected: %w", err)
	}
	petGUID := uint64(0xF140000000000001)
	petSpells := protocol.NewBuffer(8)
	petSpells.WriteU64(petGUID)
	latePetAuraTrace := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, spellGoTrace(836), {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL), Payload: base64.StdEncoding.EncodeToString(protocol.BuildAuraUpdateAll(petGUID, nil))}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_PET_SPELLS), Payload: base64.StdEncoding.EncodeToString(petSpells.Bytes())}}}
	if err := checkPostMapLoginOrder(latePetAuraTrace, 0, 1); err != nil {
		return fmt.Errorf("pet aura after post-map packets was rejected: %w", err)
	}
	latePlayerAuraTrace := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, spellGoTrace(836), {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL), Payload: base64.StdEncoding.EncodeToString(protocol.BuildAuraUpdateAll(loginGUID, nil))}}}
	if err := checkPostMapLoginOrder(latePlayerAuraTrace, 0, 1); err == nil {
		return fmt.Errorf("late player aura packet after post-map packets was not rejected")
	}
	lateMovementTrace := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}}}
	if err := checkLoginMovementOrder(lateMovementTrace, 0); err != nil {
		return fmt.Errorf("late ghost movement state was rejected: %w", err)
	}
	validCinematicOrder := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_SET_FORCED_REACTIONS)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_RESYNC_RUNES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TRIGGER_CINEMATIC)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)}}}
	if err := checkInitialCinematicOrder(validCinematicOrder, 0, 1, 2, 5); err != nil {
		return fmt.Errorf("valid first-login cinematic ordering was rejected: %w", err)
	}
	badEarlyCinematic := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TRIGGER_CINEMATIC)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_SET_FORCED_REACTIONS)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)}}}
	if err := checkInitialCinematicOrder(badEarlyCinematic, 0, 1, 3, 4); err == nil {
		return fmt.Errorf("pre-map cinematic before initial packets was not rejected")
	}
	postLoginCinematic := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_SET_FORCED_REACTIONS)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TRIGGER_CINEMATIC)}}}
	if err := checkInitialCinematicOrder(postLoginCinematic, 0, 1, 2, 3); err != nil {
		return fmt.Errorf("post-map gameplay cinematic was rejected: %w", err)
	}
	invalidMovement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}}}
	if err := checkLoginMovementOrder(invalidMovement, 0); err == nil {
		return fmt.Errorf("out-of-order movement packets were not rejected")
	}
	preTimeSyncMovement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}}}
	if err := checkLoginMovementOrder(preTimeSyncMovement, 0); err == nil {
		return fmt.Errorf("movement packet before time sync was not rejected")
	}
	if err := checkPublicPlayerValuesUpdate(); err != nil {
		return fmt.Errorf("public player values update check failed: %w", err)
	}
	if err := checkGroupFactionFields(); err != nil {
		return fmt.Errorf("group faction update check failed: %w", err)
	}
	if err := checkVisibilityAuraTransitions(); err != nil {
		return fmt.Errorf("visibility aura transition check failed: %w", err)
	}
	if err := checkMovementSpeedAuraClassification(); err != nil {
		return fmt.Errorf("movement speed aura check failed: %w", err)
	}
	if err := checkMovementSpeedUpdatePlan(); err != nil {
		return fmt.Errorf("movement speed update plan check failed: %w", err)
	}
	if err := checkPetSpellPowerCost(); err != nil {
		return fmt.Errorf("pet spell power check failed: %w", err)
	}
	if err := checkPetRuntimeTick(); err != nil {
		return fmt.Errorf("pet runtime tick check failed: %w", err)
	}
	if err := checkPetProgression(); err != nil {
		return fmt.Errorf("pet progression check failed: %w", err)
	}
	if err := checkExpectedCharacterStateDelta(); err != nil {
		return fmt.Errorf("expected character state delta check failed: %w", err)
	}
	if err := checkCollisionHeightFormula(); err != nil {
		return fmt.Errorf("collision-height formula check failed: %w", err)
	}
	if err := checkReputationFlags(); err != nil {
		return fmt.Errorf("reputation flag check failed: %w", err)
	}
	if err := checkDefaultRankSkill(); err != nil {
		return fmt.Errorf("default ranked-skill check failed: %w", err)
	}
	if err := checkRankSkillDBC(); err != nil {
		return fmt.Errorf("ranked-skill DBC check failed: %w", err)
	}
	if err := checkItemLimitCategoryDBC(); err != nil {
		return fmt.Errorf("item-limit DBC check failed: %w", err)
	}
	if err := checkSpellRankStackabilityDBC(); err != nil {
		return fmt.Errorf("spell-rank DBC check failed: %w", err)
	}
	if err := checkScalingStatDistributionDBC(); err != nil {
		return fmt.Errorf("scaling-item DBC check failed: %w", err)
	}
	if err := checkQuestRaidConfig(); err != nil {
		return fmt.Errorf("raid-quest rule check failed: %w", err)
	}
	if err := checkGroupLeaderFlag(); err != nil {
		return fmt.Errorf("group leader flag check failed: %w", err)
	}
	if err := checkItemEnchantmentDuration(); err != nil {
		return fmt.Errorf("item enchantment-duration check failed: %w", err)
	}
	if err := checkRestRateParity(); err != nil {
		return fmt.Errorf("offline rested-state check failed: %w", err)
	}
	if err := checkDailyQuestFieldStatus(); err != nil {
		return fmt.Errorf("daily quest field classification check failed: %w", err)
	}
	if err := checkSavedPetUnsummon(); err != nil {
		return fmt.Errorf("saved pet unsummon check failed: %w", err)
	}
	if err := checkCharacterCreationDefaults(); err != nil {
		return fmt.Errorf("character creation default check failed: %w", err)
	}
	if err := checkCorpseReleaseTimerBoundary(); err != nil {
		return fmt.Errorf("corpse release-timer check failed: %w", err)
	}
	if err := checkLoadedCorpseReclaimDelay(); err != nil {
		return fmt.Errorf("loaded corpse reclaim-delay check failed: %w", err)
	}
	if err := checkLoadedDeathExpireClamp(); err != nil {
		return fmt.Errorf("loaded death-expire clamp check failed: %w", err)
	}
	if err := checkLoadedCorpseConversion(); err != nil {
		return fmt.Errorf("loaded corpse conversion check failed: %w", err)
	}
	for _, compressed := range []bool{false, true} {
		for _, victimGUID := range []uint64{0, 0xF130000001} {
			event, err := loginCreateFixture(compressed, victimGUID)
			if err != nil {
				return fmt.Errorf("login create fixture build failed: %w", err)
			}
			if err := requireCreateBlock(event, 1); err != nil {
				return fmt.Errorf("login create fixture rejected: %w", err)
			}
		}
	}
	payloadChecks := []struct {
		name     string
		opcode   protocol.Opcode
		payload  []byte
		validate func(protocoltrace.Event) error
	}{
		{"verify-world", protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD, loginVerifyFixture(), requireLoginVerifyWorld},
		{"dungeon-difficulty", protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY, dungeonDifficultyFixture(), requireDungeonDifficulty},
		{"account-data-times", protocol.OpcodeSMSG_ACCOUNT_DATA_TIMES, accountDataTimesFixture(), requireAccountDataTimes},
		{"motd", protocol.OpcodeSMSG_MOTD, motdFixture(), requireMotd},
		{"instance-difficulty", protocol.OpcodeSMSG_INSTANCE_DIFFICULTY, make([]byte, 8), requireEightBytePayload},
		{"talents-info", protocol.OpcodeSMSG_TALENTS_INFO, talentsInfoFixture(), requireTalentsInfo},
		{"achievement-data", protocol.OpcodeSMSG_ALL_ACHIEVEMENT_DATA, achievementDataFixture(), requireAchievementData},
		{"initial-spells", protocol.OpcodeSMSG_INITIAL_SPELLS, initialSpellsCategoryCooldownFixture(), requireInitialSpellsCategoryCooldown},
		{"unlearn-spells", protocol.OpcodeSMSG_SEND_UNLEARN_SPELLS, []byte{0, 0, 0, 0}, requireUnlearnSpells},
		{"action-buttons", protocol.OpcodeSMSG_ACTION_BUTTONS, actionButtonsFixture(), requireActionButtons},
		{"factions", protocol.OpcodeSMSG_INITIALIZE_FACTIONS, initialFactionsFixture(), requireInitialFactions},
		{"faction-standing", protocol.OpcodeSMSG_SET_FACTION_STANDING, factionStandingFixture(), requireFactionStanding},
		{"name-query-known", protocol.OpcodeSMSG_NAME_QUERY_RESPONSE, nameQueryKnownFixture(), requireNameQueryResponse},
		{"name-query-unknown", protocol.OpcodeSMSG_NAME_QUERY_RESPONSE, nameQueryUnknownFixture(), requireNameQueryResponse},
		{"played-time", protocol.OpcodeSMSG_PLAYED_TIME, playedTimeFixture(), requirePlayedTime},
		{"contact-list", protocol.OpcodeSMSG_CONTACT_LIST, []byte{7, 0, 0, 0, 0, 0, 0, 0}, requireContactList},
		{"guild-event", protocol.OpcodeSMSG_GUILD_EVENT, []byte{2, 0}, requireGuildEvent},
		{"guild-bank-list", protocol.OpcodeSMSG_GUILD_BANK_LIST, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, requireGuildBankList},
		{"guild-roster", protocol.OpcodeSMSG_GUILD_ROSTER, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, requireGuildRoster},
		{"dance-moves", protocol.OpcodeSMSG_LEARNED_DANCE_MOVES, make([]byte, 8), func(event protocoltrace.Event) error { return requirePayloadLength(event, 8) }},
		{"feature-status", protocol.OpcodeSMSG_FEATURE_SYSTEM_STATUS, make([]byte, 2), func(event protocoltrace.Event) error { return requirePayloadLength(event, 2) }},
		{"bind-point", protocol.OpcodeSMSG_BIND_POINT_UPDATE, make([]byte, 20), func(event protocoltrace.Event) error { return requirePayloadLength(event, 20) }},
		{"time-speed", protocol.OpcodeSMSG_LOGIN_SET_TIME_SPEED, loginTimeSpeedFixture(), requireLoginTimeSpeed},
		{"login-effect", protocol.OpcodeSMSG_SPELL_GO, loginEffectFixture(), requireLoginEffect},
		{"first-login-triggered-cast", protocol.OpcodeSMSG_SPELL_GO, firstLoginCastFixture(), requireFirstLoginCast},
		{"equipment-sets", protocol.OpcodeSMSG_EQUIPMENT_SET_LIST, []byte{0, 0, 0, 0}, requireEquipmentSetList},
		{"group-list", protocol.OpcodeSMSG_GROUP_LIST, groupListFixture(), requireGroupList},
		{"world-states", protocol.OpcodeSMSG_INIT_WORLD_STATES, initWorldStatesFixture(), requireInitWorldStates},
		{"forced-reactions", protocol.OpcodeSMSG_SET_FORCED_REACTIONS, make([]byte, 4), requireForcedReactions},
		{"resync-runes", protocol.OpcodeSMSG_RESYNC_RUNES, resyncRunesFixture(), requireResyncRunes},
		{"time-sync", protocol.OpcodeSMSG_TIME_SYNC_REQ, make([]byte, 4), requireTimeSyncRequest},
		{"aura-update-all", protocol.OpcodeSMSG_AURA_UPDATE_ALL, auraUpdateFixture(), requireAuraUpdateAll},
		{"item-time-update", protocol.OpcodeSMSG_ITEM_TIME_UPDATE, protocol.BuildItemTimeUpdate(0x4000000000000106, 1234), requirePayloadLengthExact(12)},
		{"item-enchant-time-update", protocol.OpcodeSMSG_ITEM_ENCHANT_TIME_UPDATE, protocol.BuildItemEnchantTimeUpdate(0x106, 0x4000000000000106, 2, 1234), requirePayloadLengthExact(24)},
		{"pet-spells", protocol.OpcodeSMSG_PET_SPELLS, petSpellsFixture(), requirePetSpells},
		{"quest-giver-details", protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS, questGiverDetailsFixture(), requireQuestGiverDetails},
		{"quest-status-multiple", protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE, make([]byte, 4), requireQuestStatusMultiple},
		{"taxi-node-status", protocol.OpcodeSMSG_TAXINODE_STATUS, make([]byte, 9), requirePayloadLengthExact(9)},
	}
	for _, check := range payloadChecks {
		event := protocoltrace.Event{Direction: protocoltrace.ServerToClient, Opcode: uint32(check.opcode), Payload: base64.StdEncoding.EncodeToString(check.payload)}
		if err := check.validate(event); err != nil {
			return fmt.Errorf("%s payload fixture rejected: %w", check.name, err)
		}
	}
	return nil
}

func checkTransportTrajectory() error {
	points := []wotlk.TaxiSplinePoint{{MapID: 0, X: 0, Flags: 2}, {MapID: 0, X: 10, Flags: 2, Delay: 2}, {MapID: 0, X: 20, Flags: 2}}
	path, err := world.NewTransportTrajectory(points, 10, 1)
	if err != nil || path.Period() < 10000 {
		return fmt.Errorf("stop-keyed path period was invalid: path=%v error=%v", path, err)
	}
	mapID, x, _, _, _ := path.Position(7000)
	if mapID != 0 || math.Abs(float64(x-10)) > 0.001 {
		return fmt.Errorf("transport did not hold at stop frame: map=%d x=%f", mapID, x)
	}
	_, x, _, _, _ = path.Position(9000)
	if x <= 10 || x >= 20 {
		return fmt.Errorf("transport did not depart stop frame: x=%f", x)
	}
	_, x, _, _, _ = path.Position(path.Period())
	if math.Abs(float64(x)) > 0.001 {
		return fmt.Errorf("transport path did not wrap at its source period: x=%f", x)
	}
	return nil
}

func checkMapEntryEvent() error {
	runtime := scripting.NewRuntime(scripting.Config{Enabled: true})
	if err := runtime.LoadString(`RegisterMapEvent(530, 21, function(event, map, player) return event + map:GetMapId() + player:GetInstanceId() end)`); err != nil {
		return err
	}
	mapObject := &scripting.Object{Type: "Map", Fields: map[string]any{"MapId": uint32(530), "InstanceId": uint32(4)}}
	playerObject := &scripting.Object{Type: "Player", Fields: map[string]any{"MapId": uint32(530), "InstanceId": uint32(7)}}
	values, err := runtime.TriggerMapEvent(context.Background(), 530, scripting.MapEventOnPlayerEnter, mapObject, playerObject)
	if err != nil {
		return err
	}
	if len(values) != 1 || fmt.Sprint(values[0]) != "558" {
		return fmt.Errorf("map entry hook returned %v", values)
	}
	return nil
}

func checkGameDataPathResolution() error {
	if _, err := os.Stat(filepath.Join("data", "dbc", "Spell.dbc")); err != nil {
		return nil
	}
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat("bin"); err != nil {
		return nil
	}
	if err := os.Chdir("bin"); err != nil {
		return err
	}
	defer os.Chdir(workDir)
	c := config.Default()
	c.DataDir = "bin"
	c.GameDataDir = "data"
	c.ResolvePaths()
	resolved, err := filepath.Abs(c.GameDataDir)
	if err != nil {
		return err
	}
	expected, err := filepath.Abs(filepath.Join(workDir, "data"))
	if err != nil {
		return err
	}
	if filepath.Clean(resolved) != filepath.Clean(expected) {
		return fmt.Errorf("resolved game data %q, expected %q from bin working directory", resolved, expected)
	}
	return nil
}

func checkCollisionHeightFormula() error {
	if got := wotlk.CalculateCollisionHeight(false, 1, 0, 1.2, 2, 0.75); math.Abs(float64(got-1.8)) > 0.0001 {
		return fmt.Errorf("unmounted collision height was %f", got)
	}
	if got := wotlk.CalculateCollisionHeight(true, 1, 1.5, 1.2, 2, 0.75); math.Abs(float64(got-2.4)) > 0.0001 {
		return fmt.Errorf("mounted collision height was %f", got)
	}
	if got := wotlk.CalculateCollisionHeight(false, 1, 0, 0, 0, 0); math.Abs(float64(got-2.03128)) > 0.0001 {
		return fmt.Errorf("default collision height was %f", got)
	}
	return nil
}

func checkVisibilityAuraTransitions() error {
	for _, auraType := range []uint32{16, 17, 18, 19, 154} {
		if !world.AffectsPlayerVisibility(auraType) {
			return fmt.Errorf("aura type %d did not require visibility reconciliation", auraType)
		}
	}
	for _, auraType := range []uint32{78, 151, 304} {
		if world.AffectsPlayerVisibility(auraType) {
			return fmt.Errorf("aura type %d incorrectly changed player visibility", auraType)
		}
	}
	return nil
}

func checkMovementSpeedAuraClassification() error {
	for _, auraType := range []uint32{31, 32, 78, 129, 130, 171, 172, 201, 207, 208, 209, 211} {
		if !world.AffectsMovementSpeedAura(auraType) {
			return fmt.Errorf("aura type %d did not require movement-speed reconciliation", auraType)
		}
	}
	for _, auraType := range []uint32{33, 206, 207, 208, 209, 210, 211} {
		if !world.AffectsFlightSpeedAura(auraType) {
			return fmt.Errorf("aura type %d did not require flight-speed reconciliation", auraType)
		}
	}
	for _, auraType := range []uint32{201, 206, 207, 208, 209, 210, 211} {
		if world.AffectsRunSpeedAura(auraType) {
			return fmt.Errorf("flight-only aura type %d incorrectly required run-speed reconciliation", auraType)
		}
	}
	if !world.AffectsRunSpeedAura(33) {
		return fmt.Errorf("aura type 33 did not require run-speed reconciliation")
	}
	for _, auraType := range []uint32{31, 32, 58, 78, 129, 130, 201, 305} {
		if world.AffectsFlightSpeedAura(auraType) {
			return fmt.Errorf("aura type %d incorrectly required flight-speed reconciliation", auraType)
		}
	}
	for _, auraType := range []uint32{16, 18, 151, 304} {
		if world.AffectsMovementSpeedAura(auraType) {
			return fmt.Errorf("aura type %d incorrectly changed movement speed", auraType)
		}
	}
	return nil
}

func checkMovementSpeedUpdatePlan() error {
	for _, test := range []struct {
		auraType, flightAura uint32
		run, flight, canFly  bool
	}{{31, 0, true, false, false}, {33, 0, true, true, false}, {78, 0, true, false, false}, {78, 1, true, true, false}, {201, 0, false, false, true}, {206, 0, false, true, false}, {207, 0, false, true, true}, {211, 0, false, true, false}} {
		run, flight, canFly := world.ResolveMovementSpeedUpdatePlan(test.auraType, test.flightAura != 0)
		if run != test.run || flight != test.flight || canFly != test.canFly {
			return fmt.Errorf("aura type %d flight-aura=%t plan=(%t,%t,%t) want=(%t,%t,%t)", test.auraType, test.flightAura != 0, run, flight, canFly, test.run, test.flight, test.canFly)
		}
	}
	return nil
}

func checkPetSpellPowerCost() error {
	spell := wotlk.Spell{PowerType: 2, ManaCost: 12, ManaCostPct: 20}
	if cost := world.ResolvePetSpellPowerCost(spell, 100, 0); cost != 32 {
		return fmt.Errorf("focus spell cost=%d, want 32", cost)
	}
	if !world.PetCanPaySpell(spell, 32, 100, 0) || world.PetCanPaySpell(spell, 31, 100, 0) {
		return fmt.Errorf("focus affordability did not follow source cost comparison")
	}
	healthSpell := wotlk.Spell{PowerType: 0xFFFFFFFE, ManaCost: 10, ManaCostPct: 10}
	if cost := world.ResolvePetSpellPowerCost(healthSpell, 0, 200); cost != 30 {
		return fmt.Errorf("health spell cost=%d, want 30", cost)
	}
	if world.PetCanPaySpell(healthSpell, 30, 0, 200) || !world.PetCanPaySpell(healthSpell, 31, 0, 200) {
		return fmt.Errorf("health affordability did not preserve the caster at positive health")
	}
	return nil
}

func checkPetRuntimeTick() error {
	state := world.PetRuntimeState{PetType: 1, PowerType: 2, UnitFlags2: 0x00000800, Powers: [7]uint32{0, 0, 0, 0, 10000}, MaxPowers: [7]uint32{0, 0, 100, 0, 1050000}, FocusRegenTimer: 4 * time.Second, HappinessTimer: 7500 * time.Millisecond}
	next, fields := world.AdvancePetRuntime(state, 4*time.Second, 1)
	if next.Powers[2] != 24 || next.Powers[4] != 10000 || next.FocusRegenTimer != 4*time.Second || next.HappinessTimer != 3500*time.Millisecond || len(fields) != 1 || fields[27] != 24 {
		return fmt.Errorf("focus interval result=%+v fields=%v", next, fields)
	}
	next, fields = world.AdvancePetRuntime(next, 3500*time.Millisecond, 1)
	if next.Powers[4] != 9330 || next.HappinessTimer != 7500*time.Millisecond || next.FocusRegenTimer != 500*time.Millisecond || len(fields) != 1 || fields[29] != 9330 {
		return fmt.Errorf("hunter happiness interval result=%+v fields=%v", next, fields)
	}
	next.InCombat = true
	next, fields = world.AdvancePetRuntime(next, 7500*time.Millisecond, 1.5)
	if next.Powers[2] != 60 || next.Powers[4] != 8325 || next.FocusRegenTimer != 4*time.Second || next.HappinessTimer != 7500*time.Millisecond || len(fields) != 2 || fields[27] != 60 || fields[29] != 8325 {
		return fmt.Errorf("combat focus/happiness result=%+v fields=%v", next, fields)
	}
	next.Powers[2] = 99
	next.FocusRegenTimer = 4 * time.Second
	next.HappinessTimer = 7500 * time.Millisecond
	next, fields = world.AdvancePetRuntime(next, 4*time.Second, 1)
	if next.Powers[2] != 100 || next.Powers[4] != 8325 || len(fields) != 1 || fields[27] != 100 {
		return fmt.Errorf("focus cap result=%+v fields=%v", next, fields)
	}
	next.PetType = 0
	next.FocusRegenTimer = 0
	next, fields = world.AdvancePetRuntime(next, 7500*time.Millisecond, 1)
	if next.Powers[4] != 8325 || fields != nil {
		return fmt.Errorf("non-hunter happiness changed: result=%+v fields=%v", next, fields)
	}
	if config.Default().FocusRate != 1 {
		return fmt.Errorf("default Rate.Focus=%v, want 1", config.Default().FocusRate)
	}
	loaded, err := config.Load("configs/worldserver.conf.dist")
	if err != nil || loaded.FocusRate != 1 {
		return fmt.Errorf("worldserver config Rate.Focus=%v err=%v, want 1", loaded.FocusRate, err)
	}
	previous, hadPrevious := os.LookupEnv("MORENOCORE_RATE_FOCUS")
	if err := os.Setenv("MORENOCORE_RATE_FOCUS", "1.5"); err != nil {
		return err
	}
	loaded.ApplyEnv()
	if hadPrevious {
		_ = os.Setenv("MORENOCORE_RATE_FOCUS", previous)
	} else {
		_ = os.Unsetenv("MORENOCORE_RATE_FOCUS")
	}
	if loaded.FocusRate != 1.5 {
		return fmt.Errorf("environment Rate.Focus=%v, want 1.5", loaded.FocusRate)
	}
	return nil
}

func checkPetProgression() error {
	if world.PetLevelForOwner(0, 40, 60) != 60 || world.PetLevelForOwner(1, 75, 70) != 70 || world.PetLevelForOwner(1, 60, 70) != 65 || world.PetLevelForOwner(1, 67, 70) != 67 {
		return fmt.Errorf("pet level synchronization did not preserve summon and hunter-pet rules")
	}
	xpForLevel := []uint32{0, 400, 900, 1400, 2000, 2700}
	level, xp, nextXP := world.AdvanceHunterPetExperience(1, 0, 100, 4, xpForLevel[2]/20, xpForLevel)
	if level != 3 || xp != 10 || nextXP != xpForLevel[3]/20 {
		return fmt.Errorf("hunter pet multi-level XP result=(%d,%d,%d)", level, xp, nextXP)
	}
	level, xp, nextXP = world.AdvanceHunterPetExperience(level, xp, 100, 4, nextXP, xpForLevel)
	if level != 4 || xp != 0 || nextXP != xpForLevel[4]/20 {
		return fmt.Errorf("hunter pet owner-level cap result=(%d,%d,%d)", level, xp, nextXP)
	}
	level, xp, nextXP = world.AdvanceHunterPetExperience(level, 12, 100, 4, nextXP, xpForLevel)
	if level != 4 || xp != 12 || nextXP != xpForLevel[4]/20 {
		return fmt.Errorf("max-level hunter pet accepted XP: result=(%d,%d,%d)", level, xp, nextXP)
	}
	return nil
}

func checkExpectedCharacterStateDelta() error {
	before := map[string]characterTableSnapshot{
		"characters":     {Rows: 1, Columns: map[string]string{"level": "a", "position_x": "b"}},
		"character_aura": {Rows: 2, Columns: map[string]string{"remainTime": "c"}},
	}
	after := map[string]characterTableSnapshot{
		"characters":     {Rows: 1, Columns: map[string]string{"level": "a", "position_x": "d"}},
		"character_aura": {Rows: 1, Columns: map[string]string{"remainTime": "e"}},
	}
	if err := validateCharacterStateDelta(before, after); err != nil {
		return fmt.Errorf("source-expected login/logout changes rejected: %w", err)
	}
	unchangedColumns := map[string]string{"first": "same", "second": "same"}
	rowCompositionBefore := map[string]characterTableSnapshot{"characters": {Rows: 1, Digest: "before", Columns: unchangedColumns}}
	rowCompositionAfter := map[string]characterTableSnapshot{"characters": {Rows: 1, Digest: "after", Columns: unchangedColumns}}
	if err := validateCharacterStateDelta(rowCompositionBefore, rowCompositionAfter); err == nil {
		return fmt.Errorf("row composition change with unchanged column digests was accepted")
	}
	after["characters"] = characterTableSnapshot{Rows: 1, Columns: map[string]string{"level": "f", "position_x": "d"}}
	if err := validateCharacterStateDelta(before, after); err == nil {
		return fmt.Errorf("unclassified characters.level mutation was accepted")
	}
	return nil
}

func checkLoadedCorpseConversion() error {
	if !world.ShouldConvertLoadedCorpseToBones(0, 0, true) {
		return fmt.Errorf("same-map alive corpse was not selected for conversion")
	}
	if world.ShouldConvertLoadedCorpseToBones(0, 1, true) || world.ShouldConvertLoadedCorpseToBones(0, 0, false) {
		return fmt.Errorf("cross-map or dead corpse was incorrectly selected for conversion")
	}
	return nil
}

func checkLoadedCorpseReclaimDelay() error {
	if delay, ok := world.CalculateLoadedCorpseReclaimDelay(100, 130, 90, true); !ok || delay != 20 {
		return fmt.Errorf("active corpse delay=%d present=%t want 20 seconds", delay, ok)
	}
	if _, ok := world.CalculateLoadedCorpseReclaimDelay(200, 130, 90, true); ok {
		return fmt.Errorf("expired corpse still produced reclaim delay")
	}
	return nil
}

func checkLoadedDeathExpireClamp() error {
	const now int64 = 1_800_000_000
	if value := world.ClampLoadedDeathExpireTime(now, now+900); value != now+900 {
		return fmt.Errorf("death-expire exact cap=%d want %d", value, now+900)
	}
	if value := world.ClampLoadedDeathExpireTime(now, now+901); value != now+899 {
		return fmt.Errorf("death-expire overflow cap=%d want %d", value, now+899)
	}
	if value := world.ClampLoadedDeathExpireTime(now, now-1); value != now-1 {
		return fmt.Errorf("past death-expire value=%d want %d", value, now-1)
	}
	return nil
}

func checkCorpseReleaseTimerBoundary() error {
	if !world.CorpseReleaseTimerRequired(0) {
		return fmt.Errorf("continent corpse did not require release timer")
	}
	if world.CorpseReleaseTimerRequired(1) || world.CorpseReleaseTimerRequired(2) || world.CorpseReleaseTimerRequired(3) {
		return fmt.Errorf("instance corpse incorrectly required release timer")
	}
	return nil
}

func checkGroupFactionFields() error {
	cfg := config.Default()
	if cfg.AllowTwoSideInteractionGroup {
		return fmt.Errorf("cross-faction group interaction default must be disabled")
	}
	if err := cfg.Set("AllowTwoSide.Interaction.Group", "1"); err != nil || !cfg.AllowTwoSideInteractionGroup {
		return fmt.Errorf("cross-faction group configuration was not parsed: enabled=%t error=%v", cfg.AllowTwoSideInteractionGroup, err)
	}
	source := wotlk.FactionTemplate{ID: 1, Faction: 72, FactionGroup: 1, FriendGroup: 1, Enemies: [4]uint32{76}}
	recipient := wotlk.FactionTemplate{ID: 2, Faction: 76, FactionGroup: 2, FriendGroup: 2}
	bytes2, faction, override := world.ResolvePlayerGroupFactionFields(0x0000FF17, source, recipient, cfg.AllowTwoSideInteractionGroup, true, false)
	if !override || bytes2 != 0x00000800 || faction != 2 {
		return fmt.Errorf("cross-faction group values=%08x/%d override=%t", bytes2, faction, override)
	}
	for name, conditions := range map[string][3]bool{"disabled": {false, true, false}, "not-grouped": {true, false, false}, "self": {true, true, true}} {
		unchangedBytes, unchangedFaction, changed := world.ResolvePlayerGroupFactionFields(0x0000FF17, source, recipient, conditions[0], conditions[1], conditions[2])
		if changed || unchangedBytes != 0x0000FF17 || unchangedFaction != 0 {
			return fmt.Errorf("%s group values changed unexpectedly", name)
		}
	}
	friendly := source
	friendly.Enemies = [4]uint32{}
	friendly.Friends = [4]uint32{76}
	unchangedBytes, unchangedFaction, changed := world.ResolvePlayerGroupFactionFields(0x0000FF17, friendly, recipient, true, true, false)
	if changed || unchangedBytes != 0x0000FF17 || unchangedFaction != 0 {
		return fmt.Errorf("friendly faction values were overridden")
	}
	return nil
}

func checkCharacterCreationDefaults() error {
	watchedFaction, drunkenness := world.CharacterCreationDefaults()
	if watchedFaction != 0xFFFFFFFF {
		return fmt.Errorf("watched faction default=%d want -1", watchedFaction)
	}
	if drunkenness != 0 {
		return fmt.Errorf("drunkenness default=%d want 0", drunkenness)
	}
	return nil
}

func checkReputationFlags() error {
	visible := world.MergeReputationFlags(0x01, 0, 42000)
	if visible&0x01 == 0 {
		return fmt.Errorf("default visible faction was cleared by an empty database flag set")
	}
	peace := world.MergeReputationFlags(0x11, 0x02, 42000)
	if peace&0x02 != 0 {
		return fmt.Errorf("peace-forced faction was marked at war")
	}
	hostile := world.MergeReputationFlags(0x01, 0, -42000)
	if hostile&0x02 == 0 {
		return fmt.Errorf("hostile faction was not forced at war")
	}
	peaceForcedHostile := world.MergeReputationFlags(0x11, 0x02, -5000)
	if peaceForcedHostile&0x02 != 0 {
		return fmt.Errorf("peace-forced hostile faction ignored source rank threshold")
	}
	baseAdjusted := world.MergeReputationFlags(0x01, 0, -7000)
	if baseAdjusted&0x02 == 0 {
		return fmt.Errorf("base-adjusted hostile faction was not at war")
	}
	return nil
}

func checkDefaultRankSkill() error {
	step, value, max, ok := world.ResolveDefaultRankSkill(3, 225, 0, 1, 21)
	if !ok || step != 3 || value != 1 || max != 225 {
		return fmt.Errorf("ranked default skill=%d/%d/%d valid=%t want 3/1/225", step, value, max, ok)
	}
	step, value, max, ok = world.ResolveDefaultRankSkill(2, 150, wotlk.SkillFlagAlwaysMaxValue, 1, 21)
	if !ok || step != 2 || value != 150 || max != 150 {
		return fmt.Errorf("always-max ranked skill=%d/%d/%d valid=%t want 2/150/150", step, value, max, ok)
	}
	step, value, max, ok = world.ResolveDefaultRankSkill(3, 225, 0, 6, 10)
	if !ok || step != 3 || value != 45 || max != 225 {
		return fmt.Errorf("death-knight ranked skill=%d/%d/%d valid=%t want 3/45/225", step, value, max, ok)
	}
	if _, _, _, ok := world.ResolveDefaultRankSkill(0, 225, 0, 6, 10); ok {
		return fmt.Errorf("zero-rank default skill was initialized")
	}
	return nil
}

func checkRankSkillDBC() error {
	store := wotlk.NewStore(filepath.Join(config.Default().GameDataDir, "dbc"))
	file, err := store.File("SkillRaceClassInfo")
	if err != nil {
		return err
	}
	for index := 0; index < file.Records(); index++ {
		record, err := file.Record(index)
		if err != nil {
			continue
		}
		skillID, skillErr := record.Uint32(1)
		raceMask, raceErr := record.Uint32(2)
		classMask, classErr := record.Uint32(3)
		flags, flagsErr := record.Uint32(4)
		tierID, tierErr := record.Uint32(6)
		if skillErr != nil || raceErr != nil || classErr != nil || flagsErr != nil || tierErr != nil || skillID == 0 || tierID == 0 {
			continue
		}
		race, class := firstSkillMaskMember(raceMask), firstSkillMaskMember(classMask)
		rangeType, found, rangeErr := store.SkillRangeType(skillID, race, class)
		if rangeErr != nil || !found || rangeType != wotlk.SkillRangeRank {
			continue
		}
		tierMax, found, tierErr := store.SkillTierValue(skillID, race, class, 1)
		if tierErr != nil || !found || tierMax == 0 {
			return fmt.Errorf("ranked skill %d tier %d did not load rank 1", skillID, tierID)
		}
		step, value, max, valid := world.ResolveDefaultRankSkill(1, tierMax, flags, 1, 1)
		if !valid || step != 1 || max != tierMax || value < 1 || value > max {
			return fmt.Errorf("ranked skill %d resolved %d/%d/%d valid=%t", skillID, step, value, max, valid)
		}
		return nil
	}
	return fmt.Errorf("SkillRaceClassInfo.dbc contains no rank-tier skill usable by a test race/class")
}

func checkItemLimitCategoryDBC() error {
	store := wotlk.NewStore(filepath.Join(config.Default().GameDataDir, "dbc"))
	file, err := store.File("ItemLimitCategory")
	if err != nil {
		return err
	}
	for index := 0; index < file.Records(); index++ {
		record, err := file.Record(index)
		if err != nil {
			continue
		}
		id, err := record.Uint32(0)
		if err != nil || id == 0 {
			continue
		}
		entry, found, err := store.ItemLimitCategory(id)
		if err != nil || !found || entry.ID != id || entry.Flags > 1 {
			return fmt.Errorf("item limit category %d decoded as %+v found=%t error=%v", id, entry, found, err)
		}
		return nil
	}
	return fmt.Errorf("ItemLimitCategory.dbc contains no usable records")
}

func checkSpellRankStackabilityDBC() error {
	store := wotlk.NewStore(filepath.Join(config.Default().GameDataDir, "dbc"))
	file, err := store.File("Spell")
	if err != nil {
		return err
	}
	stackable, nonStackable := false, false
	for index := 0; index < file.Records(); index++ {
		record, err := file.Record(index)
		if err != nil {
			continue
		}
		id, err := record.Uint32(0)
		if err != nil || id == 0 {
			continue
		}
		value, found, err := store.SpellStackableWithRanks(id)
		if err != nil {
			return err
		}
		if found && value {
			stackable = true
		}
		if found && !value {
			nonStackable = true
		}
		if stackable && nonStackable {
			return nil
		}
	}
	return fmt.Errorf("Spell.dbc did not yield both stackable and non-stackable rank categories")
}

func checkScalingStatDistributionDBC() error {
	store := wotlk.NewStore(filepath.Join(config.Default().GameDataDir, "dbc"))
	file, err := store.File("ScalingStatDistribution")
	if err != nil {
		return err
	}
	for index := 0; index < file.Records(); index++ {
		record, err := file.Record(index)
		if err != nil {
			continue
		}
		id, err := record.Uint32(0)
		if err != nil || id == 0 {
			continue
		}
		if _, found, err := store.ScalingStatDistributionMaxLevel(id); err != nil || !found {
			return fmt.Errorf("scaling stat distribution %d did not decode: found=%t error=%v", id, found, err)
		}
		return nil
	}
	return fmt.Errorf("ScalingStatDistribution.dbc contains no usable records")
}

func checkQuestRaidConfig() error {
	const env = "MORENOCORE_QUEST_IGNORE_RAID"
	oldValue, wasSet := os.LookupEnv(env)
	defer func() {
		if wasSet {
			_ = os.Setenv(env, oldValue)
		} else {
			_ = os.Unsetenv(env)
		}
	}()
	if err := os.Setenv(env, "true"); err != nil {
		return err
	}
	cfg := config.Default()
	cfg.ApplyEnv()
	if !cfg.QuestIgnoreRaid {
		return fmt.Errorf("Quests.IgnoreRaid environment mapping was not applied")
	}
	if !world.QuestAllowedInRaid(world.QuestTypeRaid, 0, true, false) || world.QuestAllowedInRaid(world.QuestTypeRaid10, world.RaidDifficulty25ManMask, true, false) || !world.QuestAllowedInRaid(world.QuestTypeRaid25, world.RaidDifficulty25ManMask, true, false) || world.QuestAllowedInRaid(0, 0, true, false) || !world.QuestAllowedInRaid(0, 0, true, true) {
		return fmt.Errorf("source raid quest type/difficulty rules differ")
	}
	return nil
}

func firstSkillMaskMember(mask uint32) uint8 {
	if mask == 0 {
		return 1
	}
	for index := uint32(0); index < 32; index++ {
		if mask&(uint32(1)<<index) != 0 {
			return uint8(index + 1)
		}
	}
	return 0
}

func checkGroupLeaderFlag() error {
	flags := uint32(0x00800002)
	if result := world.UpdatePlayerGroupLeaderFlag(flags, true); result != flags|0x01 {
		return fmt.Errorf("leader player flags=%08x want %08x", result, flags|0x01)
	}
	if result := world.UpdatePlayerGroupLeaderFlag(flags|0x01, false); result != flags {
		return fmt.Errorf("non-leader player flags=%08x want %08x", result, flags)
	}
	return nil
}

func checkItemEnchantmentDuration() error {
	for _, location := range [][2]int64{{0, 15}, {0, 23}, {0, 67}, {812345, 2}} {
		if !world.ShouldTrackItemEnchantmentDuration(location[0], location[1], 1, 60000) {
			return fmt.Errorf("timed enchant at bag/slot %d/%d was not scheduled", location[0], location[1])
		}
	}
	if world.ShouldTrackItemEnchantmentDuration(0, 15, 0, 60000) || world.ShouldTrackItemEnchantmentDuration(0, 15, 1, 0) {
		return fmt.Errorf("empty or permanent enchant was scheduled")
	}
	return nil
}

func checkRestRateParity() error {
	cfg := config.Default()
	if cfg.RestOfflineInTavernOrCityRate != 1 || cfg.RestOfflineInWildernessRate != 1 {
		return fmt.Errorf("offline rest defaults=%v/%v want 1/1", cfg.RestOfflineInTavernOrCityRate, cfg.RestOfflineInWildernessRate)
	}
	if err := cfg.Set("Rate.Rest.Offline.InTavernOrCity", "0.5"); err != nil || cfg.RestOfflineInTavernOrCityRate != 0.5 {
		return fmt.Errorf("tavern rest rate=%v error=%v", cfg.RestOfflineInTavernOrCityRate, err)
	}
	if err := cfg.Set("Rate.Rest.Offline.InWilderness", "2"); err != nil || cfg.RestOfflineInWildernessRate != 2 {
		return fmt.Errorf("wilderness rest rate=%v error=%v", cfg.RestOfflineInWildernessRate, err)
	}
	if !world.IsPlayerRestingForLogout(0x20) || world.IsPlayerRestingForLogout(0x08) {
		return fmt.Errorf("logout resting state does not follow PLAYER_FLAGS_RESTING")
	}
	return nil
}

func checkDailyQuestFieldStatus() error {
	if fieldQuest, dungeonFinder := world.DailyQuestFieldStatus(0x1000, 0); !fieldQuest || dungeonFinder {
		return fmt.Errorf("ordinary daily quest classification=%t/%t", fieldQuest, dungeonFinder)
	}
	if fieldQuest, dungeonFinder := world.DailyQuestFieldStatus(0x1000, 0x08); fieldQuest || !dungeonFinder {
		return fmt.Errorf("Dungeon Finder daily classification=%t/%t", fieldQuest, dungeonFinder)
	}
	if fieldQuest, dungeonFinder := world.DailyQuestFieldStatus(0, 0); fieldQuest || dungeonFinder {
		return fmt.Errorf("non-daily quest classification=%t/%t", fieldQuest, dungeonFinder)
	}
	return nil
}

func checkSavedPetUnsummon() error {
	for _, state := range [][5]uint32{{100, 0, 0, 0, 1}, {0, 0, 0, 0, 0}, {100, 0x10, 0, 0, 0}, {100, 0, 0x08000000, 0, 0}, {100, 0, 0, 1, 0}} {
		if !world.ShouldTemporarilyUnsummonSavedPet(state[0], state[1], state[2], state[3], state[4] != 0) {
			return fmt.Errorf("saved pet state %v was not deferred", state)
		}
	}
	if world.ShouldTemporarilyUnsummonSavedPet(100, 0, 0, 0, false) {
		return fmt.Errorf("permanent pet of a live, unmounted owner was deferred")
	}
	return nil
}

func checkPublicPlayerValuesUpdate() error {
	if !world.IsPlayerFieldVisibleToRecipient(158, false, true) || world.IsPlayerFieldVisibleToRecipient(158, false, false) || !world.IsPlayerFieldVisibleToRecipient(158, true, false) {
		return fmt.Errorf("party quest field 158 recipient visibility is incorrect")
	}
	fields := map[int]uint32{18: 0x12345678, 19: 0x40000000, 24: 100, 67: 12345, 74: 0x00020000, 1197: 0x00000002, 1229: 0x60000000, 151: 42, 152: 3, 1020: 4}
	public := make(map[int]uint32, len(fields))
	for field, value := range fields {
		if world.IsPlayerFieldPublic(field) {
			public[field] = value
		}
	}
	if _, ok := public[1020]; ok {
		return fmt.Errorf("private player field 1020 was retained")
	}
	for _, field := range []int{18, 19, 24, 67, 74, 151, 152} {
		if _, ok := public[field]; !ok {
			return fmt.Errorf("public player field %d was filtered", field)
		}
	}
	payload := valuesUpdateFixture(0x4000000000000106, public)
	reader := protocol.NewReader(payload)
	kind, err := reader.ReadU8()
	if err != nil || kind != protocol.UpdateValues {
		return fmt.Errorf("values update kind=%d error=%v", kind, err)
	}
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("values update GUID: %w", err)
	}
	_, values, err := readUpdateValues(reader)
	if err != nil {
		return err
	}
	if len(values) != len(public) {
		return fmt.Errorf("values update field count=%d want=%d", len(values), len(public))
	}
	if _, ok := values[1020]; ok {
		return fmt.Errorf("values update contains private field 1020")
	}
	return nil
}

func valuesUpdateFixture(guid uint64, fields map[int]uint32) []byte {
	maxField := 0
	for field := range fields {
		if field > maxField {
			maxField = field
		}
	}
	maskBlocks := maxField/32 + 1
	mask := make([]uint32, maskBlocks)
	for field := range fields {
		mask[field/32] |= uint32(1) << uint(field%32)
	}
	buf := protocol.NewBuffer(32 + len(fields)*4)
	buf.WriteU8(protocol.UpdateValues)
	buf.WritePackedGUID(guid)
	buf.WriteU8(uint8(maskBlocks))
	for _, block := range mask {
		buf.WriteU32(block)
	}
	for field := 0; field <= maxField; field++ {
		if mask[field/32]&(uint32(1)<<uint(field%32)) != 0 {
			buf.WriteU32(fields[field])
		}
	}
	return buf.Bytes()
}

func loginVerifyFixture() []byte {
	buf := protocol.NewBuffer(20)
	buf.WriteI32(0)
	for range 4 {
		buf.WriteF32(0)
	}
	return buf.Bytes()
}

func loginTimeSpeedFixture() []byte {
	buf := protocol.NewBuffer(12)
	buf.WriteU32(0)
	buf.WriteF32(float32(0.01666667 * 30))
	buf.WriteU32(0)
	return buf.Bytes()
}

func dungeonDifficultyFixture() []byte {
	buf := protocol.NewBuffer(12)
	buf.WriteU32(0)
	buf.WriteU32(1)
	buf.WriteU32(0)
	return buf.Bytes()
}

func accountDataTimesFixture() []byte {
	buf := protocol.NewBuffer(29)
	buf.WriteU32(0)
	buf.WriteU8(1)
	buf.WriteU32(0xEA)
	for index := uint32(0); index < 8; index++ {
		if 0xEA&(1<<index) != 0 {
			buf.WriteU32(0)
		}
	}
	return buf.Bytes()
}

func motdFixture() []byte {
	buf := protocol.NewBuffer(12)
	buf.WriteU32(2)
	buf.WriteCString("one")
	buf.WriteCString("two")
	return buf.Bytes()
}

func talentsInfoFixture() []byte {
	buf := protocol.NewBuffer(20)
	buf.WriteU8(0)
	buf.WriteU32(0)
	buf.WriteU8(1)
	buf.WriteU8(0)
	buf.WriteU8(0)
	buf.WriteU8(6)
	for range 6 {
		buf.WriteU16(0)
	}
	return buf.Bytes()
}

func achievementDataFixture() []byte {
	buf := protocol.NewBuffer(8)
	buf.WriteU32(0xFFFFFFFF)
	buf.WriteU32(0xFFFFFFFF)
	return buf.Bytes()
}

func factionStandingFixture() []byte {
	buf := protocol.NewBuffer(17)
	buf.WriteF32(0)
	buf.WriteU8(1)
	buf.WriteU32(1)
	buf.WriteU32(72)
	buf.WriteU32(42999)
	return buf.Bytes()
}

func nameQueryKnownFixture() []byte {
	buf := protocol.NewBuffer(32)
	buf.WritePackedGUID(26)
	buf.WriteU8(0)
	buf.WriteCString("Denveous")
	buf.WriteU8(0)
	buf.WriteU8(4)
	buf.WriteU8(1)
	buf.WriteU8(2)
	buf.WriteU8(0)
	return buf.Bytes()
}

func nameQueryUnknownFixture() []byte {
	buf := protocol.NewBuffer(10)
	buf.WritePackedGUID(999999)
	buf.WriteU8(1)
	return buf.Bytes()
}

func playedTimeFixture() []byte {
	buf := protocol.NewBuffer(9)
	buf.WriteU32(3600)
	buf.WriteU32(120)
	buf.WriteU8(1)
	return buf.Bytes()
}

func questGiverDetailsFixture() []byte {
	packet := protocol.NewBuffer(256)
	packet.WriteU64(1)
	packet.WriteU64(0)
	packet.WriteU32(100)
	packet.WriteCString("Quest")
	packet.WriteCString("Details")
	packet.WriteCString("Objectives")
	packet.WriteU8(1)
	packet.WriteU32(0)
	packet.WriteU32(0)
	packet.WriteU8(0)
	packet.WriteU32(0)
	packet.WriteU32(0)
	for index := 0; index < 10+15; index++ {
		packet.WriteU32(0)
	}
	packet.WriteI32(0)
	return packet.Bytes()
}

func groupListFixture() []byte {
	packet := protocol.NewBuffer(64)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU64(uint64(0x1F50)<<48 | 1)
	packet.WriteU32(0)
	packet.WriteU32(1)
	packet.WriteCString("Noradinia")
	packet.WriteU64(126)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU64(127)
	packet.WriteU8(3)
	packet.WriteU64(0)
	packet.WriteU8(2)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	return packet.Bytes()
}

func loginEffectFixture() []byte {
	power := uint32(777)
	target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: 0x106}
	return protocol.BuildSpellGoWithPower(0x106, 0x106, 0, 836, 0x40901, 123, []uint64{0x106}, nil, target, &power)
}

func firstLoginCastFixture() []byte {
	target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: 0x106}
	power := uint32(777)
	return protocol.BuildSpellGoWithPower(0x106, 0x106, 0, 668, 0x40901, 123, []uint64{0x106}, nil, target, &power)
}

func initWorldStatesFixture() []byte {
	buf := protocol.NewBuffer(14)
	buf.WriteI32(0)
	buf.WriteI32(0)
	buf.WriteI32(0)
	buf.WriteU16(0)
	return buf.Bytes()
}

func resyncRunesFixture() []byte {
	buf := protocol.NewBuffer(16)
	buf.WriteU32(6)
	for range 6 {
		buf.WriteU8(0)
		buf.WriteU8(0)
	}
	return buf.Bytes()
}

func auraUpdateFixture() []byte {
	return protocol.BuildAuraUpdateAll(0x106, []protocol.AuraUpdateRecord{{CasterGUID: 0x106, Slot: 0, SpellID: 836, EffectMask: 0x01, Positive: true, MaxDurationMs: 1000, DurationMs: 500, CasterLevel: 10, StackCount: 2}})
}

func petSpellsFixture() []byte {
	buf := protocol.NewBuffer(64)
	buf.WriteU64(0xF140000000000106)
	buf.WriteU16(23)
	buf.WriteU32(0)
	buf.WriteU8(1)
	buf.WriteU8(1)
	buf.WriteU16(0)
	for range 10 {
		buf.WriteU32(0)
	}
	buf.WriteU8(2)
	buf.WriteU32(6307 | 0x81000000)
	buf.WriteU32(7799 | 0x81000000)
	buf.WriteU8(0)
	return buf.Bytes()
}

func actionButtonsFixture() []byte {
	buf := protocol.NewBuffer(1 + 144*4)
	buf.WriteU8(1)
	for range 144 {
		buf.WriteU32(0)
	}
	return buf.Bytes()
}

func initialFactionsFixture() []byte {
	buf := protocol.NewBuffer(4 + 128*5)
	buf.WriteU32(128)
	for range 128 {
		buf.WriteU8(0)
		buf.WriteU32(0)
	}
	return buf.Bytes()
}

func initialSpellsCategoryCooldownFixture() []byte {
	buf := protocol.NewBuffer(29)
	buf.WriteU8(0)
	buf.WriteU16(1)
	buf.WriteU32(133)
	buf.WriteU16(0)
	buf.WriteU16(1)
	buf.WriteU32(133)
	buf.WriteU16(0)
	buf.WriteU16(5)
	buf.WriteU32(0)
	buf.WriteU32(60000)
	return buf.Bytes()
}

func loginCreateFixture(compressed bool, victimGUID uint64) (protocoltrace.Event, error) {
	item := protocol.NewBuffer(64)
	item.WriteU8(protocol.UpdateCreateObject2)
	item.WritePackedGUID(0x4001)
	item.WriteU8(1)
	item.WriteU16(0x0010)
	item.WriteU32(1)
	item.WriteU8(2)
	item.WriteU32(0x00000006)
	item.WriteU32(0)
	item.WriteU32(3)
	item.WriteU32(0)
	player := protocol.NewBuffer(512)
	player.WriteU8(protocol.UpdateCreateObject2)
	player.WritePackedGUID(1)
	player.WriteU8(4)
	player.WriteU16(world.PlayerCreateUpdateFlags(true, victimGUID != 0))
	player.WriteU32(0)
	player.WriteU16(0)
	player.WriteU32(1)
	player.WriteF32(1)
	player.WriteF32(2)
	player.WriteF32(3)
	player.WriteF32(0)
	player.WriteU32(0)
	for range 9 {
		player.WriteF32(1)
	}
	if victimGUID != 0 {
		player.WritePackedGUID(victimGUID)
	}
	mask := make([]uint32, 42)
	values := map[int]uint32{0: 1, 2: 0x19, 4: math.Float32bits(1), 23: 0x01020304, 24: 100, 32: 100, 54: 10, 59: 8, 67: 123, 68: 123}
	for field := range values {
		mask[field/32] |= 1 << uint(field%32)
	}
	player.WriteU8(uint8(len(mask)))
	for _, block := range mask {
		player.WriteU32(block)
	}
	for field := 0; field < len(mask)*32; field++ {
		if mask[field/32]&(1<<uint(field%32)) != 0 {
			player.WriteU32(values[field])
		}
	}
	body := protocol.NewBuffer(4 + item.Len() + player.Len())
	body.WriteU32(2)
	body.Write(item.Bytes())
	body.Write(player.Bytes())
	payload := body.Bytes()
	opcode := uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)
	if compressed {
		var compressedBody bytes.Buffer
		writer := zlib.NewWriter(&compressedBody)
		if _, err := writer.Write(payload); err != nil {
			return protocoltrace.Event{}, err
		}
		if err := writer.Close(); err != nil {
			return protocoltrace.Event{}, err
		}
		encoded := protocol.NewBuffer(4 + compressedBody.Len())
		encoded.WriteU32(uint32(len(payload)))
		encoded.Write(compressedBody.Bytes())
		payload = encoded.Bytes()
		opcode = uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT)
	}
	return protocoltrace.Event{Direction: protocoltrace.ServerToClient, Opcode: opcode, Payload: base64.StdEncoding.EncodeToString(payload)}, nil
}

func findOrderedLoginStages(trace protocoltrace.Trace, start int, stages []loginStage) ([]int, error) {
	if start < 0 || start >= len(trace.Events) {
		return nil, fmt.Errorf("login start index %d is out of range", start)
	}
	playerGUID := uint64(0)
	for _, stage := range stages {
		if stage.Name == "player create update" {
			var err error
			playerGUID, err = loginPlayerGUID(trace.Events[start])
			if err != nil {
				return nil, err
			}
			break
		}
	}
	positions := make([]int, len(stages))
	position := start
	for stageIndex, stage := range stages {
		for index := start + 1; index < position; index++ {
			event := trace.Events[index]
			matched, err := loginStageMatches(stage, event, playerGUID)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", stage.Name, err)
			}
			if event.Direction == protocoltrace.ServerToClient && matched {
				return nil, fmt.Errorf("%s appeared before an earlier login stage", stage.Name)
			}
		}
		found := -1
		for index := position + 1; index < len(trace.Events); index++ {
			event := trace.Events[index]
			matched, err := loginStageMatches(stage, event, playerGUID)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", stage.Name, err)
			}
			if event.Direction == protocoltrace.ServerToClient && matched {
				found = index
				break
			}
			if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
				break
			}
		}
		if found < 0 {
			return nil, fmt.Errorf("missing or out-of-order %s", stage.Name)
		}
		positions[stageIndex] = found
		position = found
	}
	return positions, nil
}

func loginStageMatches(stage loginStage, event protocoltrace.Event, playerGUID uint64) (bool, error) {
	if stage.Name != "player create update" {
		return stage.Match(event.Opcode), nil
	}
	if event.Opcode != uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) && event.Opcode != uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		return false, nil
	}
	return containsPlayerCreate(event, playerGUID)
}

func loginPlayerGUID(event protocoltrace.Event) (uint64, error) {
	payload, err := eventPayload(event)
	if err != nil {
		return 0, err
	}
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil || guid == 0 {
		return 0, fmt.Errorf("CMSG_PLAYER_LOGIN has an invalid GUID")
	}
	return guid, nil
}

func checkLogin(trace protocoltrace.Trace, start int) error {
	if err := rejectPreVerifyAchievementPackets(trace, start); err != nil {
		return err
	}
	if err := checkLoginMovementOrder(trace, start); err != nil {
		return err
	}
	playerGUID, err := loginPlayerGUID(trace.Events[start])
	if err != nil {
		return err
	}
	stages := []loginStage{
		{"MSG_SET_DUNGEON_DIFFICULTY", exact(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY)},
		{"SMSG_LOGIN_VERIFY_WORLD", exact(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD)},
		{"SMSG_ACCOUNT_DATA_TIMES", exact(protocol.OpcodeSMSG_ACCOUNT_DATA_TIMES)},
		{"SMSG_FEATURE_SYSTEM_STATUS", exact(protocol.OpcodeSMSG_FEATURE_SYSTEM_STATUS)},
		{"SMSG_MOTD", exact(protocol.OpcodeSMSG_MOTD)},
		{"SMSG_LEARNED_DANCE_MOVES", exact(protocol.OpcodeSMSG_LEARNED_DANCE_MOVES)},
		{"SMSG_CONTACT_LIST", exact(protocol.OpcodeSMSG_CONTACT_LIST)},
		{"SMSG_BIND_POINT_UPDATE", exact(protocol.OpcodeSMSG_BIND_POINT_UPDATE)},
		{"SMSG_TALENTS_INFO", exact(protocol.OpcodeSMSG_TALENTS_INFO)},
		{"SMSG_INSTANCE_DIFFICULTY", exact(protocol.OpcodeSMSG_INSTANCE_DIFFICULTY)},
		{"SMSG_INITIAL_SPELLS", exact(protocol.OpcodeSMSG_INITIAL_SPELLS)},
		{"SMSG_SEND_UNLEARN_SPELLS", exact(protocol.OpcodeSMSG_SEND_UNLEARN_SPELLS)},
		{"SMSG_ACTION_BUTTONS", exact(protocol.OpcodeSMSG_ACTION_BUTTONS)},
		{"SMSG_INITIALIZE_FACTIONS", exact(protocol.OpcodeSMSG_INITIALIZE_FACTIONS)},
		{"SMSG_ALL_ACHIEVEMENT_DATA", exact(protocol.OpcodeSMSG_ALL_ACHIEVEMENT_DATA)},
		{"SMSG_EQUIPMENT_SET_LIST", exact(protocol.OpcodeSMSG_EQUIPMENT_SET_LIST)},
		{"SMSG_LOGIN_SET_TIME_SPEED", exact(protocol.OpcodeSMSG_LOGIN_SET_TIME_SPEED)},
		{"SMSG_SET_FORCED_REACTIONS", exact(protocol.OpcodeSMSG_SET_FORCED_REACTIONS)},
		{"player create update", func(opcode uint32) bool {
			return opcode == uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) || opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT)
		}},
		{"SMSG_INIT_WORLD_STATES", exact(protocol.OpcodeSMSG_INIT_WORLD_STATES)},
		{"SMSG_TIME_SYNC_REQ", exact(protocol.OpcodeSMSG_TIME_SYNC_REQ)},
	}
	verifyIndex := -1
	forcedReactionsIndex := -1
	playerCreateIndex := -1
	positions, err := findOrderedLoginStages(trace, start, stages)
	if err != nil {
		return err
	}
	for stageIndex, stage := range stages {
		found := positions[stageIndex]
		if stage.Name == "player create update" {
			if err := requireCreateBlock(trace.Events[found], playerGUID); err != nil {
				return err
			}
			playerCreateIndex = found
		}
		var validate func(protocoltrace.Event) error
		switch stage.Name {
		case "MSG_SET_DUNGEON_DIFFICULTY":
			validate = requireDungeonDifficulty
		case "SMSG_LOGIN_VERIFY_WORLD":
			validate = requireLoginVerifyWorld
		case "SMSG_ACCOUNT_DATA_TIMES":
			validate = requireAccountDataTimes
		case "SMSG_MOTD":
			validate = requireMotd
		case "SMSG_TALENTS_INFO":
			validate = requireTalentsInfo
		case "SMSG_ALL_ACHIEVEMENT_DATA":
			validate = requireAchievementData
		case "SMSG_SET_FACTION_STANDING":
			validate = requireFactionStanding
		case "SMSG_NAME_QUERY_RESPONSE":
			validate = requireNameQueryResponse
		case "SMSG_PLAYED_TIME":
			validate = requirePlayedTime
		case "SMSG_LEARNED_DANCE_MOVES":
			validate = func(event protocoltrace.Event) error { return requirePayloadLength(event, 8) }
		case "SMSG_FEATURE_SYSTEM_STATUS":
			validate = func(event protocoltrace.Event) error { return requirePayloadLength(event, 2) }
		case "SMSG_BIND_POINT_UPDATE":
			validate = func(event protocoltrace.Event) error { return requirePayloadLength(event, 20) }
		case "SMSG_CONTACT_LIST":
			validate = requireContactList
		case "SMSG_INSTANCE_DIFFICULTY":
			validate = requireEightBytePayload
		case "SMSG_INITIAL_SPELLS":
			validate = requireInitialSpells
		case "SMSG_SEND_UNLEARN_SPELLS":
			validate = requireUnlearnSpells
		case "SMSG_ACTION_BUTTONS":
			validate = requireActionButtons
		case "SMSG_INITIALIZE_FACTIONS":
			validate = requireInitialFactions
		case "SMSG_SET_FORCED_REACTIONS":
			validate = requireForcedReactions
		case "SMSG_EQUIPMENT_SET_LIST":
			validate = requireEquipmentSetList
		case "SMSG_INIT_WORLD_STATES":
			validate = requireInitWorldStates
		case "SMSG_TIME_SYNC_REQ":
			validate = requireTimeSyncRequest
		}
		if validate != nil {
			if err := validate(trace.Events[found]); err != nil {
				return fmt.Errorf("%s: %w", stage.Name, err)
			}
		}
		if stage.Name == "SMSG_LOGIN_SET_TIME_SPEED" {
			if err := requireLoginTimeSpeed(trace.Events[found]); err != nil {
				return err
			}
		}
		if stage.Name == "SMSG_LOGIN_VERIFY_WORLD" {
			verifyIndex = found
		}
		if stage.Name == "SMSG_SET_FORCED_REACTIONS" {
			forcedReactionsIndex = found
		}
	}
	if err := checkOptionalLoginPayloads(trace, start); err != nil {
		return err
	}
	if err := checkPostMapLoginOrder(trace, start, playerCreateIndex); err != nil {
		return err
	}
	if err := checkOptionalPreMapRuneOrder(trace, start, playerCreateIndex); err != nil {
		return err
	}
	if err := checkPreMapGuildLoginOrder(trace, start, playerCreateIndex); err != nil {
		return err
	}
	if err := checkInitialCinematicOrder(trace, start, verifyIndex, forcedReactionsIndex, playerCreateIndex); err != nil {
		return err
	}
	return nil
}

func checkInitialCinematicOrder(trace protocoltrace.Trace, start, verifyIndex, forcedReactionsIndex, playerCreateIndex int) error {
	if verifyIndex < 0 || forcedReactionsIndex < 0 || playerCreateIndex < 0 {
		return fmt.Errorf("cinematic ordering is missing a required login boundary")
	}
	runeIndex := -1
	for index := forcedReactionsIndex + 1; index < playerCreateIndex; index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ServerToClient && event.Opcode == uint32(protocol.OpcodeSMSG_RESYNC_RUNES) {
			runeIndex = index
			break
		}
	}
	for index := start + 1; index < playerCreateIndex; index++ {
		event := trace.Events[index]
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_TRIGGER_CINEMATIC) {
			continue
		}
		if index < verifyIndex {
			return fmt.Errorf("SMSG_TRIGGER_CINEMATIC was sent before SMSG_LOGIN_VERIFY_WORLD")
		}
		if index < forcedReactionsIndex || runeIndex >= 0 && index < runeIndex {
			return fmt.Errorf("SMSG_TRIGGER_CINEMATIC was sent before the pre-map login packets completed")
		}
	}
	return nil
}

func checkOptionalPreMapRuneOrder(trace protocoltrace.Trace, start, playerCreateIndex int) error {
	end := playerCreateIndex
	if end < 0 || end > len(trace.Events) {
		end = len(trace.Events)
	}
	forcedIndex, runeIndex := -1, -1
	for index := start + 1; index < end; index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			break
		}
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_SET_FORCED_REACTIONS) && forcedIndex < 0 {
			forcedIndex = index
		}
		if event.Opcode != uint32(protocol.OpcodeSMSG_RESYNC_RUNES) {
			continue
		}
		if runeIndex < 0 {
			runeIndex = index
		}
	}
	if runeIndex >= 0 && forcedIndex >= 0 && runeIndex < forcedIndex {
		return fmt.Errorf("SMSG_RESYNC_RUNES arrived before SMSG_SET_FORCED_REACTIONS")
	}
	return nil
}

func checkPreMapGuildLoginOrder(trace protocoltrace.Trace, start, playerCreateIndex int) error {
	const (
		guildEventMOTD     uint8 = 2
		guildEventSignedOn uint8 = 12
	)
	end := playerCreateIndex
	if end < 0 || end >= len(trace.Events) {
		end = len(trace.Events)
	}
	motdIndex, danceIndex := -1, end
	for index := start + 1; index < end; index++ {
		event := trace.Events[index]
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_MOTD) && motdIndex < 0 {
			motdIndex = index
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_LEARNED_DANCE_MOVES) && danceIndex == end {
			danceIndex = index
		}
	}
	lastStage := -1
	for index := start + 1; index < end; index++ {
		event := trace.Events[index]
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		stage := -1
		switch event.Opcode {
		case uint32(protocol.OpcodeSMSG_GUILD_EVENT):
			payload, err := eventPayload(event)
			if err != nil {
				return err
			}
			if len(payload) == 0 {
				return fmt.Errorf("guild event payload is empty")
			}
			switch payload[0] {
			case guildEventMOTD:
				stage = 0
			case guildEventSignedOn:
				stage = 3
			default:
				continue
			}
		case uint32(protocol.OpcodeSMSG_GUILD_BANK_LIST):
			stage = 1
		case uint32(protocol.OpcodeSMSG_GUILD_ROSTER):
			stage = 2
		default:
			continue
		}
		if motdIndex < 0 || index <= motdIndex || index >= danceIndex {
			return fmt.Errorf("guild packet %s is outside the source MOTD-to-dance window", opcodeName(event.Opcode))
		}
		if stage < lastStage {
			return fmt.Errorf("guild packet %s arrived after a later guild login stage", opcodeName(event.Opcode))
		}
		lastStage = stage
	}
	return nil
}

func checkPostMapLoginOrder(trace protocoltrace.Trace, start, playerCreateIndex int) error {
	if start < 0 || start >= len(trace.Events) || playerCreateIndex <= start || playerCreateIndex >= len(trace.Events) {
		return fmt.Errorf("post-map login order has no player create boundary")
	}
	playerGUID, err := loginPlayerGUID(trace.Events[start])
	if err != nil {
		return err
	}
	for index := start + 1; index < playerCreateIndex; index++ {
		event := trace.Events[index]
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_SPELL_GO) {
			continue
		}
		spellID, err := spellGoSpellID(event)
		if err != nil {
			return fmt.Errorf("pre-map SMSG_SPELL_GO: %w", err)
		}
		if spellID == 836 {
			return fmt.Errorf("login spell 836 was sent before the player create/map-entry boundary")
		}
	}
	petGUIDs := make(map[uint64]struct{})
	for index := playerCreateIndex + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			break
		}
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_PET_SPELLS) {
			continue
		}
		payload, err := eventPayload(event)
		if err != nil {
			return fmt.Errorf("SMSG_PET_SPELLS: %w", err)
		}
		if len(payload) < 8 {
			continue
		}
		reader := protocol.NewReader(payload)
		guid, err := reader.ReadU64()
		if err != nil {
			return fmt.Errorf("SMSG_PET_SPELLS pet GUID: %w", err)
		}
		if guid != 0 {
			petGUIDs[guid] = struct{}{}
		}
	}
	order := map[uint32]int{
		uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES):          0,
		uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ):              1,
		uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL):            3,
		uint32(protocol.OpcodeSMSG_ITEM_ENCHANT_TIME_UPDATE):   4,
		uint32(protocol.OpcodeSMSG_ITEM_TIME_UPDATE):           5,
		uint32(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE): 6,
		uint32(protocol.OpcodeSMSG_TAXINODE_STATUS):            7,
		uint32(protocol.OpcodeMSG_SET_RAID_DIFFICULTY):         8,
		uint32(protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS):  9,
		uint32(protocol.OpcodeSMSG_GROUP_LIST):                 10,
		uint32(protocol.OpcodeSMSG_PET_SPELLS):                 11,
	}
	seen := make(map[int]struct{}, len(order))
	last := -1
	loginEffectCount := 0
	for index := playerCreateIndex + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			break
		}
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL) && len(event.Payload) > 0 {
			payload, err := eventPayload(event)
			if err != nil {
				return fmt.Errorf("SMSG_AURA_UPDATE_ALL: %w", err)
			}
			reader := protocol.NewReader(payload)
			guid, err := reader.ReadPackedGUID()
			if err != nil {
				return fmt.Errorf("SMSG_AURA_UPDATE_ALL target GUID: %w", err)
			}
			if _, ok := petGUIDs[guid]; ok {
				continue
			}
		}
		stage, ok := order[event.Opcode]
		if event.Opcode == uint32(protocol.OpcodeSMSG_SPELL_GO) {
			spellID, err := spellGoSpellID(event)
			if err != nil {
				return fmt.Errorf("SMSG_SPELL_GO: %w", err)
			}
			if spellID != 836 {
				continue
			}
			loginEffectCount++
			if loginEffectCount > 1 {
				return fmt.Errorf("login spell 836 was sent more than once")
			}
			if err := requireLoginEffectForPlayer(event, playerGUID); err != nil {
				return err
			}
			stage, ok = 2, true
		}
		if !ok {
			continue
		}
		if _, ok := seen[stage]; ok {
			continue
		}
		if stage < last {
			return fmt.Errorf("post-map packet %s arrived after a later login stage", opcodeName(event.Opcode))
		}
		seen[stage] = struct{}{}
		last = stage
	}
	if loginEffectCount != 1 {
		return fmt.Errorf("post-map login effect spell 836 count=%d, want 1", loginEffectCount)
	}
	return nil
}

func spellGoSpellID(event protocoltrace.Event) (uint32, error) {
	payload, err := eventPayload(event)
	if err != nil {
		return 0, err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadPackedGUID(); err != nil {
		return 0, err
	}
	if _, err := reader.ReadPackedGUID(); err != nil {
		return 0, err
	}
	if _, err := reader.ReadU8(); err != nil {
		return 0, err
	}
	return reader.ReadU32()
}

func checkOptionalLoginPayloads(trace protocoltrace.Trace, start int) error {
	for index := start + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			break
		}
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		var validate func(protocoltrace.Event) error
		switch event.Opcode {
		case uint32(protocol.OpcodeSMSG_RESYNC_RUNES):
			validate = requireResyncRunes
		case uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL):
			validate = requireAuraUpdateAll
		case uint32(protocol.OpcodeSMSG_ITEM_TIME_UPDATE):
			validate = requirePayloadLengthExact(12)
		case uint32(protocol.OpcodeSMSG_ITEM_ENCHANT_TIME_UPDATE):
			validate = requirePayloadLengthExact(24)
		case uint32(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE):
			validate = requireQuestStatusMultiple
		case uint32(protocol.OpcodeSMSG_TAXINODE_STATUS):
			validate = requirePayloadLengthExact(9)
		case uint32(protocol.OpcodeSMSG_PET_SPELLS):
			validate = requirePetSpells
		case uint32(protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS):
			validate = requireQuestGiverDetails
		case uint32(protocol.OpcodeSMSG_GROUP_LIST):
			validate = requireGroupList
		case uint32(protocol.OpcodeSMSG_GUILD_EVENT):
			validate = requireGuildEvent
		case uint32(protocol.OpcodeSMSG_GUILD_BANK_LIST):
			validate = requireGuildBankList
		case uint32(protocol.OpcodeSMSG_GUILD_ROSTER):
			validate = requireGuildRoster
		case uint32(protocol.OpcodeSMSG_SET_FACTION_STANDING):
			validate = requireFactionStanding
		case uint32(protocol.OpcodeSMSG_NAME_QUERY_RESPONSE):
			validate = requireNameQueryResponse
		case uint32(protocol.OpcodeSMSG_PLAYED_TIME):
			validate = requirePlayedTime
		}
		if validate != nil {
			if err := validate(event); err != nil {
				return fmt.Errorf("%s: %w", opcodeName(event.Opcode), err)
			}
		}
	}
	return nil
}

func requireAchievementData(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	for {
		id, err := reader.ReadU32()
		if err != nil {
			return fmt.Errorf("achievement ID is truncated: %w", err)
		}
		if id == 0xFFFFFFFF {
			break
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("achievement %d packed date is truncated: %w", id, err)
		}
	}
	for {
		criteria, err := reader.ReadU32()
		if err != nil {
			return fmt.Errorf("achievement criteria ID is truncated: %w", err)
		}
		if criteria == 0xFFFFFFFF {
			break
		}
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("achievement criteria %d counter is truncated: %w", criteria, err)
		}
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("achievement criteria %d player GUID is truncated: %w", criteria, err)
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("achievement criteria %d flags are truncated: %w", criteria, err)
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("achievement criteria %d packed date is truncated: %w", criteria, err)
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("achievement criteria %d elapsed time is truncated: %w", criteria, err)
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("achievement criteria %d creation time is truncated: %w", criteria, err)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected achievement-data bytes=%d", reader.Remaining())
	}
	return nil
}

func requireTalentsInfo(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	pet, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("talents-info pet flag is truncated: %w", err)
	}
	if pet != 0 && pet != 1 {
		return fmt.Errorf("invalid talents-info pet flag=%d", pet)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("talents-info points are truncated: %w", err)
	}
	count, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("talents-info talent count is truncated: %w", err)
	}
	if pet != 0 {
		for index := uint8(0); index < count; index++ {
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("pet talent %d ID is truncated: %w", index, err)
			}
			if _, err := reader.ReadU8(); err != nil {
				return fmt.Errorf("pet talent %d rank is truncated: %w", index, err)
			}
		}
		if reader.Remaining() != 0 {
			return fmt.Errorf("unexpected pet talents-info bytes=%d", reader.Remaining())
		}
		return nil
	}
	activeSpec, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("talents-info active spec is truncated: %w", err)
	}
	if count > 2 || count == 0 && activeSpec != 0 || count > 0 && activeSpec >= count {
		return fmt.Errorf("invalid talents-info spec count=%d active=%d", count, activeSpec)
	}
	for spec := uint8(0); spec < count; spec++ {
		talentCount, err := reader.ReadU8()
		if err != nil {
			return fmt.Errorf("talent spec %d count is truncated: %w", spec, err)
		}
		for index := uint8(0); index < talentCount; index++ {
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("talent spec %d talent %d ID is truncated: %w", spec, index, err)
			}
			if _, err := reader.ReadU8(); err != nil {
				return fmt.Errorf("talent spec %d talent %d rank is truncated: %w", spec, index, err)
			}
		}
		glyphCount, err := reader.ReadU8()
		if err != nil {
			return fmt.Errorf("talent spec %d glyph count is truncated: %w", spec, err)
		}
		if glyphCount > 6 {
			return fmt.Errorf("talent spec %d glyph count=%d exceeds client slots", spec, glyphCount)
		}
		for glyph := uint8(0); glyph < glyphCount; glyph++ {
			if _, err := reader.ReadU16(); err != nil {
				return fmt.Errorf("talent spec %d glyph %d is truncated: %w", spec, glyph, err)
			}
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected talents-info bytes=%d", reader.Remaining())
	}
	return nil
}

func requireDungeonDifficulty(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("dungeon difficulty is truncated: %w", err)
	}
	marker, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("dungeon difficulty marker is truncated: %w", err)
	}
	inGroup, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("dungeon difficulty group flag is truncated: %w", err)
	}
	if marker != 1 || inGroup != 0 || reader.Remaining() != 0 {
		return fmt.Errorf("invalid dungeon difficulty payload marker=%d inGroup=%d remaining=%d", marker, inGroup, reader.Remaining())
	}
	return nil
}

func requireAccountDataTimes(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("account-data server time is truncated: %w", err)
	}
	version, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("account-data version is truncated: %w", err)
	}
	mask, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("account-data mask is truncated: %w", err)
	}
	if version != 1 || mask != 0xEA {
		return fmt.Errorf("invalid character account-data header version=%d mask=0x%08x", version, mask)
	}
	for index := uint32(0); index < 8; index++ {
		if mask&(1<<index) != 0 {
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("account-data timestamp %d is truncated: %w", index, err)
			}
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected account-data bytes=%d", reader.Remaining())
	}
	return nil
}

func requireMotd(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("motd line count is truncated: %w", err)
	}
	for index := uint32(0); index < count; index++ {
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("motd line %d is truncated: %w", index, err)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected motd bytes=%d", reader.Remaining())
	}
	return nil
}

func requireLoginTimeSpeed(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("login time field is truncated: %w", err)
	}
	speed, err := reader.ReadF32()
	if err != nil {
		return fmt.Errorf("login time speed is truncated: %w", err)
	}
	holiday, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("login holiday offset is truncated: %w", err)
	}
	if speed != float32(0.01666667*30) || holiday != 0 || reader.Remaining() != 0 {
		return fmt.Errorf("invalid login time-speed payload speed=%v holiday=%d remaining=%d", speed, holiday, reader.Remaining())
	}
	return nil
}

func requireEquipmentSetList(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("equipment-set count is truncated: %w", err)
	}
	if count > 10 {
		return fmt.Errorf("equipment-set count=%d exceeds client limit", count)
	}
	for index := uint32(0); index < count; index++ {
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("equipment-set %d GUID is truncated: %w", index, err)
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("equipment-set %d index is truncated: %w", index, err)
		}
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("equipment-set %d name is truncated: %w", index, err)
		}
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("equipment-set %d icon is truncated: %w", index, err)
		}
		for slot := 0; slot < 19; slot++ {
			if _, err := reader.ReadPackedGUID(); err != nil {
				return fmt.Errorf("equipment-set %d item %d is truncated: %w", index, slot, err)
			}
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected equipment-set payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireContactList(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("contact flags are truncated: %w", err)
	}
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("contact count is truncated: %w", err)
	}
	for index := uint32(0); index < count; index++ {
		if _, err := reader.ReadU64(); err != nil {
			return fmt.Errorf("contact %d GUID is truncated: %w", index, err)
		}
		flags, err := reader.ReadU32()
		if err != nil {
			return fmt.Errorf("contact %d flags are truncated: %w", index, err)
		}
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("contact %d note is truncated: %w", index, err)
		}
		if flags&1 != 0 {
			status, err := reader.ReadU8()
			if err != nil {
				return fmt.Errorf("contact %d status is truncated: %w", index, err)
			}
			if status != 0 {
				if _, err := reader.Read(12); err != nil {
					return fmt.Errorf("contact %d online fields are truncated: %w", index, err)
				}
			}
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected contact payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireGuildEvent(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	eventType, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("guild event type is truncated: %w", err)
	}
	count, err := reader.ReadU8()
	if err != nil || count > 3 {
		return fmt.Errorf("invalid guild event parameter count=%d", count)
	}
	for index := uint8(0); index < count; index++ {
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("guild event parameter %d is truncated: %w", index, err)
		}
	}
	if eventType == 3 || eventType == 4 || eventType == 12 || eventType == 13 {
		if _, err := reader.ReadU64(); err != nil {
			return fmt.Errorf("guild event GUID is truncated: %w", err)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected guild event payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireGuildBankList(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("guild bank money is truncated: %w", err)
	}
	tab, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("guild bank tab is truncated: %w", err)
	}
	if _, err := reader.ReadI32(); err != nil {
		return fmt.Errorf("guild bank withdrawals are truncated: %w", err)
	}
	fullUpdate, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("guild bank full-update flag is truncated: %w", err)
	}
	if tab == 0 && fullUpdate != 0 {
		tabCount, err := reader.ReadU8()
		if err != nil {
			return fmt.Errorf("guild bank tab count is truncated: %w", err)
		}
		for index := uint8(0); index < tabCount; index++ {
			if _, err := reader.ReadCString(); err != nil {
				return fmt.Errorf("guild bank tab %d name is truncated: %w", index, err)
			}
			if _, err := reader.ReadCString(); err != nil {
				return fmt.Errorf("guild bank tab %d icon is truncated: %w", index, err)
			}
		}
	}
	itemCount, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("guild bank item count is truncated: %w", err)
	}
	for index := uint8(0); index < itemCount; index++ {
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("guild bank item %d slot is truncated: %w", index, err)
		}
		itemID, err := reader.ReadU32()
		if err != nil {
			return fmt.Errorf("guild bank item %d ID is truncated: %w", index, err)
		}
		if itemID == 0 {
			continue
		}
		if _, err := reader.ReadI32(); err != nil {
			return fmt.Errorf("guild bank item %d flags are truncated: %w", index, err)
		}
		randomPropertyID, err := reader.ReadI32()
		if err != nil {
			return fmt.Errorf("guild bank item %d random property is truncated: %w", index, err)
		}
		if randomPropertyID != 0 {
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("guild bank item %d suffix seed is truncated: %w", index, err)
			}
		}
		if _, err := reader.ReadI32(); err != nil {
			return fmt.Errorf("guild bank item %d count is truncated: %w", index, err)
		}
		if _, err := reader.ReadI32(); err != nil {
			return fmt.Errorf("guild bank item %d enchantment is truncated: %w", index, err)
		}
		charges, err := reader.ReadU8()
		if err != nil {
			return fmt.Errorf("guild bank item %d charges are truncated: %w", index, err)
		}
		_ = charges
		socketCount, err := reader.ReadU8()
		if err != nil {
			return fmt.Errorf("guild bank item %d socket count is truncated: %w", index, err)
		}
		for socket := uint8(0); socket < socketCount; socket++ {
			if _, err := reader.Read(5); err != nil {
				return fmt.Errorf("guild bank item %d socket %d is truncated: %w", index, socket, err)
			}
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected guild bank payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireGuildRoster(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	members, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("guild roster member count is truncated: %w", err)
	}
	if _, err := reader.ReadCString(); err != nil {
		return fmt.Errorf("guild roster welcome text is truncated: %w", err)
	}
	if _, err := reader.ReadCString(); err != nil {
		return fmt.Errorf("guild roster info text is truncated: %w", err)
	}
	ranks, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("guild roster rank count is truncated: %w", err)
	}
	for index := uint32(0); index < ranks; index++ {
		if _, err := reader.Read(56); err != nil {
			return fmt.Errorf("guild roster rank %d is truncated: %w", index, err)
		}
	}
	for index := uint32(0); index < members; index++ {
		if _, err := reader.ReadU64(); err != nil {
			return fmt.Errorf("guild roster member %d GUID is truncated: %w", index, err)
		}
		status, err := reader.ReadU8()
		if err != nil {
			return fmt.Errorf("guild roster member %d status is truncated: %w", index, err)
		}
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("guild roster member %d name is truncated: %w", index, err)
		}
		if _, err := reader.Read(11); err != nil {
			return fmt.Errorf("guild roster member %d fields are truncated: %w", index, err)
		}
		if status == 0 {
			if _, err := reader.ReadF32(); err != nil {
				return fmt.Errorf("guild roster member %d last-save is truncated: %w", index, err)
			}
		}
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("guild roster member %d note is truncated: %w", index, err)
		}
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("guild roster member %d officer note is truncated: %w", index, err)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected guild roster payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireLoginEffect(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("login effect caster GUID is truncated: %w", err)
	}
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("login effect caster-unit GUID is truncated: %w", err)
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("login effect cast ID is truncated: %w", err)
	}
	spellID, err := reader.ReadU32()
	if err != nil || spellID != 836 {
		return fmt.Errorf("login effect spell ID=%d, want 836", spellID)
	}
	flags, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("login effect cast flags are truncated: %w", err)
	}
	if flags&protocol.SpellCastFlagPowerLeftSelf == 0 || flags&protocol.SpellCastFlagNoGCD == 0 || flags&0x100 == 0 || flags&0x1 == 0 {
		return fmt.Errorf("login effect cast flags=0x%08x, want unknown-9/pending/power-left-self", flags)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("login effect cast time is truncated: %w", err)
	}
	hitCount, err := reader.ReadU8()
	if err != nil || hitCount != 1 {
		return fmt.Errorf("login effect hit count=%d, want 1", hitCount)
	}
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("login effect hit target is truncated: %w", err)
	}
	missCount, err := reader.ReadU8()
	if err != nil || missCount != 0 {
		return fmt.Errorf("login effect miss count=%d, want 0", missCount)
	}
	target, err := protocol.ReadSpellTargetData(reader)
	if err != nil {
		return fmt.Errorf("login effect target is truncated: %w", err)
	}
	if target.Flags != protocol.SpellTargetFlagUnit || target.UnitGUID != 0x106 {
		return fmt.Errorf("login effect target flags/guid=0x%08x/0x%x", target.Flags, target.UnitGUID)
	}
	power, err := reader.ReadU32()
	if err != nil || power != 777 {
		return fmt.Errorf("login effect remaining power=%d, want 777", power)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected login effect payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireLoginEffectForPlayer(event protocoltrace.Event, playerGUID uint64) error {
	if playerGUID == 0 {
		return fmt.Errorf("login effect player GUID is zero")
	}
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	casterGUID, err := reader.ReadPackedGUID()
	if err != nil || casterGUID != playerGUID {
		return fmt.Errorf("login effect caster GUID=0x%x, want player 0x%x", casterGUID, playerGUID)
	}
	casterUnitGUID, err := reader.ReadPackedGUID()
	if err != nil || casterUnitGUID != playerGUID {
		return fmt.Errorf("login effect caster-unit GUID=0x%x, want player 0x%x", casterUnitGUID, playerGUID)
	}
	castID, err := reader.ReadU8()
	if err != nil || castID != 0 {
		return fmt.Errorf("login effect cast ID=%d, want 0", castID)
	}
	spellID, err := reader.ReadU32()
	if err != nil || spellID != 836 {
		return fmt.Errorf("login effect spell ID=%d, want 836", spellID)
	}
	flags, err := reader.ReadU32()
	if err != nil || flags&protocol.SpellCastFlagPowerLeftSelf == 0 || flags&protocol.SpellCastFlagNoGCD == 0 || flags&0x100 == 0 || flags&0x1 == 0 {
		return fmt.Errorf("login effect cast flags=0x%08x, want unknown-9/pending/power-left-self", flags)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("login effect cast time is truncated: %w", err)
	}
	hitCount, err := reader.ReadU8()
	if err != nil || hitCount != 1 {
		return fmt.Errorf("login effect hit count=%d, want 1", hitCount)
	}
	hitGUID, err := reader.ReadU64()
	if err != nil || hitGUID != playerGUID {
		return fmt.Errorf("login effect hit target GUID=0x%x, want player 0x%x", hitGUID, playerGUID)
	}
	missCount, err := reader.ReadU8()
	if err != nil || missCount != 0 {
		return fmt.Errorf("login effect miss count=%d, want 0", missCount)
	}
	target, err := protocol.ReadSpellTargetData(reader)
	if err != nil || target.Flags != protocol.SpellTargetFlagUnit || target.UnitGUID != playerGUID {
		return fmt.Errorf("login effect target flags/GUID=0x%08x/0x%x, want self unit 0x%x", target.Flags, target.UnitGUID, playerGUID)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("login effect remaining power is truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected login effect payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireFirstLoginCast(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("first-login caster GUID is truncated: %w", err)
	}
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("first-login caster-unit GUID is truncated: %w", err)
	}
	castID, err := reader.ReadU8()
	if err != nil || castID != 0 {
		return fmt.Errorf("first-login cast ID=%d, want 0", castID)
	}
	spellID, err := reader.ReadU32()
	if err != nil || spellID == 0 || spellID == 836 {
		return fmt.Errorf("first-login spell ID=%d is invalid", spellID)
	}
	flags, err := reader.ReadU32()
	if err != nil || flags != 0x40901 {
		return fmt.Errorf("first-login cast flags=0x%08x, want unknown-9/pending/power-left-self/no-GCD", flags)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("first-login cast time is truncated: %w", err)
	}
	hitCount, err := reader.ReadU8()
	if err != nil || hitCount != 1 {
		return fmt.Errorf("first-login hit count=%d, want 1", hitCount)
	}
	targetGUID, err := reader.ReadU64()
	if err != nil || targetGUID != 0x106 {
		return fmt.Errorf("first-login hit target=0x%x, want 0x106", targetGUID)
	}
	missCount, err := reader.ReadU8()
	if err != nil || missCount != 0 {
		return fmt.Errorf("first-login miss count=%d, want 0", missCount)
	}
	target, err := protocol.ReadSpellTargetData(reader)
	if err != nil || target.Flags != protocol.SpellTargetFlagUnit || target.UnitGUID != 0x106 {
		return fmt.Errorf("first-login target is invalid: %w", err)
	}
	power, err := reader.ReadU32()
	if err != nil || power != 777 {
		return fmt.Errorf("first-login remaining power=%d, want 777", power)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected first-login cast payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireInitWorldStates(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	for range 3 {
		if _, err := reader.ReadI32(); err != nil {
			return fmt.Errorf("world-state map/zone/area is truncated: %w", err)
		}
	}
	count, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("world-state count is truncated: %w", err)
	}
	if _, err := reader.Read(int(count) * 8); err != nil {
		return fmt.Errorf("world-state entries are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected world-state payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireForcedReactions(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("forced-reaction count is truncated: %w", err)
	}
	if _, err := reader.Read(int(count) * 8); err != nil {
		return fmt.Errorf("forced-reaction entries are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected forced-reaction payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireResyncRunes(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count != 6 {
		return fmt.Errorf("rune count=%d, want 6", count)
	}
	if _, err := reader.Read(int(count) * 2); err != nil {
		return fmt.Errorf("rune entries are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected rune payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireTimeSyncRequest(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if len(payload) != 4 {
		return fmt.Errorf("time-sync payload length=%d, want 4", len(payload))
	}
	reader := protocol.NewReader(payload)
	counter, err := reader.ReadU32()
	if err != nil || counter != 0 {
		return fmt.Errorf("initial time-sync counter=%d, want 0", counter)
	}
	return nil
}

func requirePayloadLengthExact(length int) func(protocoltrace.Event) error {
	return func(event protocoltrace.Event) error {
		return requirePayloadLength(event, length)
	}
}

func requireAuraUpdateAll(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("aura target GUID is truncated: %w", err)
	}
	count := 0
	for reader.Remaining() > 0 {
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("aura slot is truncated: %w", err)
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("aura spell is truncated: %w", err)
		}
		flags, err := reader.ReadU8()
		if err != nil {
			return fmt.Errorf("aura flags are truncated: %w", err)
		}
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("aura caster level is truncated: %w", err)
		}
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("aura stack count is truncated: %w", err)
		}
		if flags&protocol.AuraFlagCaster == 0 {
			if _, err := reader.ReadPackedGUID(); err != nil {
				return fmt.Errorf("aura caster GUID is truncated: %w", err)
			}
		}
		if flags&protocol.AuraFlagDuration != 0 {
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("aura max duration is truncated: %w", err)
			}
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("aura duration is truncated: %w", err)
			}
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("aura update has no records")
	}
	return nil
}

func requireQuestStatusMultiple(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("quest-status count is truncated: %w", err)
	}
	if _, err := reader.Read(int(count) * 9); err != nil {
		return fmt.Errorf("quest-status entries are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected quest-status payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireQuestGiverDetails(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("quest giver GUID is truncated: %w", err)
	}
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("quest inform GUID is truncated: %w", err)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("quest ID is truncated: %w", err)
	}
	for _, field := range []string{"title", "details", "objectives"} {
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("quest %s is truncated: %w", field, err)
		}
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("quest auto-launch flag is truncated: %w", err)
	}
	if _, err := reader.Read(9); err != nil {
		return fmt.Errorf("quest header fields are truncated: %w", err)
	}
	choiceCount, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("quest choice count is truncated: %w", err)
	}
	if _, err := reader.Read(int(choiceCount) * 12); err != nil {
		return fmt.Errorf("quest choice items are truncated: %w", err)
	}
	rewardCount, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("quest reward count is truncated: %w", err)
	}
	if _, err := reader.Read(int(rewardCount) * 12); err != nil {
		return fmt.Errorf("quest reward items are truncated: %w", err)
	}
	if _, err := reader.Read(40 + 60); err != nil {
		return fmt.Errorf("quest reward fields are truncated: %w", err)
	}
	emoteCount, err := reader.ReadI32()
	if err != nil || emoteCount < 0 {
		return fmt.Errorf("quest emote count=%d", emoteCount)
	}
	if _, err := reader.Read(int(emoteCount) * 8); err != nil {
		return fmt.Errorf("quest emotes are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected quest-details payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requirePetSpells(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if len(payload) == 8 {
		return nil
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("pet GUID is truncated: %w", err)
	}
	if _, err := reader.ReadU16(); err != nil {
		return fmt.Errorf("pet family is truncated: %w", err)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("pet duration is truncated: %w", err)
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("pet react state is truncated: %w", err)
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("pet command state is truncated: %w", err)
	}
	if _, err := reader.ReadU16(); err != nil {
		return fmt.Errorf("pet flags are truncated: %w", err)
	}
	if _, err := reader.Read(10 * 4); err != nil {
		return fmt.Errorf("pet action bar is truncated: %w", err)
	}
	spellCount, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("pet spell count is truncated: %w", err)
	}
	if _, err := reader.Read(int(spellCount) * 4); err != nil {
		return fmt.Errorf("pet spell list is truncated: %w", err)
	}
	cooldownCount, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("pet cooldown count is truncated: %w", err)
	}
	if _, err := reader.Read(int(cooldownCount) * 14); err != nil {
		return fmt.Errorf("pet cooldown list is truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected pet-spell payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireLoginVerifyWorld(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadI32(); err != nil {
		return fmt.Errorf("map ID is truncated: %w", err)
	}
	for index := 0; index < 4; index++ {
		value, readErr := reader.ReadF32()
		if readErr != nil {
			return fmt.Errorf("position field %d is truncated: %w", index, readErr)
		}
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("position field %d is not finite", index)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected login verify payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireEightBytePayload(event protocoltrace.Event) error {
	return requirePayloadLength(event, 8)
}

func requirePayloadLength(event protocoltrace.Event, want int) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if len(payload) != want {
		return fmt.Errorf("payload length=%d, want %d", len(payload), want)
	}
	return nil
}

func requireInitialSpellsCategoryCooldown(event protocoltrace.Event) error {
	if err := requireInitialSpells(event); err != nil {
		return err
	}
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU8(); err != nil {
		return err
	}
	spellCount, err := reader.ReadU16()
	if err != nil {
		return err
	}
	if _, err := reader.Read(int(spellCount) * 6); err != nil {
		return err
	}
	cooldownCount, err := reader.ReadU16()
	if err != nil || cooldownCount != 1 {
		return fmt.Errorf("category cooldown count=%d, want 1", cooldownCount)
	}
	if _, err := reader.ReadU32(); err != nil {
		return err
	}
	if _, err := reader.ReadU16(); err != nil {
		return err
	}
	if _, err := reader.ReadU16(); err != nil {
		return err
	}
	spellDuration, err := reader.ReadU32()
	if err != nil {
		return err
	}
	categoryDuration, err := reader.ReadU32()
	if err != nil {
		return err
	}
	if spellDuration != 0 || categoryDuration == 0 || reader.Remaining() != 0 {
		return fmt.Errorf("invalid category-only cooldown spell=%d category=%d remaining=%d", spellDuration, categoryDuration, reader.Remaining())
	}
	return nil
}

func requireInitialSpells(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("initial spell flags are truncated: %w", err)
	}
	count, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("initial spell count is truncated: %w", err)
	}
	for index := uint16(0); index < count; index++ {
		if _, err := reader.Read(6); err != nil {
			return fmt.Errorf("initial spell %d is truncated: %w", index, err)
		}
	}
	cooldowns, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("initial cooldown count is truncated: %w", err)
	}
	for index := uint16(0); index < cooldowns; index++ {
		if _, err := reader.Read(16); err != nil {
			return fmt.Errorf("initial cooldown %d is truncated: %w", index, err)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected initial-spell payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireUnlearnSpells(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("unlearn count is truncated: %w", err)
	}
	if _, err := reader.Read(int(count) * 4); err != nil {
		return fmt.Errorf("unlearn spell list is truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected unlearn payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireActionButtons(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	state := byte(0)
	if len(payload) > 0 {
		state = payload[0]
	}
	if len(payload) != 1+144*4 || state != 1 {
		return fmt.Errorf("action-button payload length/state=%d/%d, want 577/1", len(payload), state)
	}
	return nil
}

func requirePlayedTime(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if len(payload) != 9 {
		return fmt.Errorf("played-time payload length=%d, want 9", len(payload))
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("played-time total is truncated: %w", err)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("played-time level is truncated: %w", err)
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("played-time trigger is truncated: %w", err)
	}
	return nil
}

func requireNameQueryResponse(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("name-query GUID is truncated: %w", err)
	}
	known, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("name-query result is truncated: %w", err)
	}
	if known != 0 {
		if known != 1 || reader.Remaining() != 0 {
			return fmt.Errorf("invalid unknown name-query response bytes=%d", reader.Remaining())
		}
		return nil
	}
	if _, err := reader.ReadCString(); err != nil {
		return fmt.Errorf("name-query name is truncated: %w", err)
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("name-query realm field is truncated: %w", err)
	}
	for field := 0; field < 3; field++ {
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("name-query character field %d is truncated: %w", field, err)
		}
	}
	declined, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("name-query declined flag is truncated: %w", err)
	}
	if declined > 1 {
		return fmt.Errorf("invalid name-query declined flag=%d", declined)
	}
	if declined != 0 {
		for index := 0; index < 5; index++ {
			if _, err := reader.ReadCString(); err != nil {
				return fmt.Errorf("name-query declined name %d is truncated: %w", index, err)
			}
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected name-query bytes=%d", reader.Remaining())
	}
	return nil
}

func requireFactionStanding(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadF32(); err != nil {
		return fmt.Errorf("faction-standing increase is truncated: %w", err)
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("faction-standing increase flag is truncated: %w", err)
	}
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("faction-standing count is truncated: %w", err)
	}
	if _, err := reader.Read(int(count) * 8); err != nil {
		return fmt.Errorf("faction-standing entries are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected faction-standing bytes=%d", reader.Remaining())
	}
	return nil
}

func requireInitialFactions(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if len(payload) < 4 {
		return fmt.Errorf("faction payload is truncated")
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count != 128 {
		return fmt.Errorf("faction count=%d, want 128", count)
	}
	if _, err := reader.Read(128 * 5); err != nil {
		return fmt.Errorf("faction state list is truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected faction payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireGroupList(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	groupType, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("group type is truncated: %w", err)
	}
	if _, err := reader.Read(3); err != nil {
		return fmt.Errorf("group slot state is truncated: %w", err)
	}
	if groupType&0x08 != 0 {
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("LFG status is truncated: %w", err)
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("LFG dungeon is truncated: %w", err)
		}
	}
	groupGUID, err := reader.ReadU64()
	if err != nil {
		return fmt.Errorf("group GUID is truncated: %w", err)
	}
	if groupGUID>>48 != 0x1F50 || groupGUID&0xFFFFFFFF == 0 {
		return fmt.Errorf("group GUID=0x%016x, want HighGuid::Group 0x1F50 with nonzero low word", groupGUID)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("group counter is truncated: %w", err)
	}
	memberCount, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("group member count is truncated: %w", err)
	}
	for index := uint32(0); index < memberCount; index++ {
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("group member %d name is truncated: %w", index, err)
		}
		if _, err := reader.ReadU64(); err != nil {
			return fmt.Errorf("group member %d GUID is truncated: %w", index, err)
		}
		if _, err := reader.Read(4); err != nil {
			return fmt.Errorf("group member %d state is truncated: %w", index, err)
		}
	}
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("group leader GUID is truncated: %w", err)
	}
	if memberCount > 0 {
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("group loot method is truncated: %w", err)
		}
		if _, err := reader.ReadU64(); err != nil {
			return fmt.Errorf("group master looter GUID is truncated: %w", err)
		}
		if _, err := reader.Read(4); err != nil {
			return fmt.Errorf("group difficulty state is truncated: %w", err)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected group-list payload bytes=%d", reader.Remaining())
	}
	return nil
}

func rejectPreVerifyAchievementPackets(trace protocoltrace.Trace, start int) error {
	for index := start + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ServerToClient && event.Opcode == uint32(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD) {
			return nil
		}
		if event.Direction == protocoltrace.ServerToClient && (event.Opcode == uint32(protocol.OpcodeSMSG_CRITERIA_UPDATE) || event.Opcode == uint32(protocol.OpcodeSMSG_ACHIEVEMENT_EARNED)) {
			return fmt.Errorf("%s was sent before SMSG_LOGIN_VERIFY_WORLD", opcodeName(event.Opcode))
		}
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			return fmt.Errorf("login ended before SMSG_LOGIN_VERIFY_WORLD")
		}
	}
	return fmt.Errorf("missing SMSG_LOGIN_VERIFY_WORLD")
}

func checkLoginMovementOrder(trace protocoltrace.Trace, start int) error {
	order := map[uint32]int{
		uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK):           0,
		uint32(protocol.OpcodeSMSG_MOVE_FEATHER_FALL):         1,
		uint32(protocol.OpcodeSMSG_MOVE_SET_HOVER):            2,
		uint32(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY):          3,
		uint32(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE): 4,
		uint32(protocol.OpcodeSMSG_FORCE_MOVE_ROOT):           5,
		uint32(protocol.OpcodeSMSG_MULTIPLE_MOVES):            6,
		uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL):           7,
	}
	last := -1
	seen := make(map[int]struct{}, len(order))
	timeSyncSeen := false
	for index := start + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			break
		}
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE) {
			break
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ) {
			timeSyncSeen = true
			continue
		}
		stage, ok := order[event.Opcode]
		if !ok {
			continue
		}
		if !timeSyncSeen {
			return fmt.Errorf("movement packet %s arrived before SMSG_TIME_SYNC_REQ", opcodeName(event.Opcode))
		}
		if _, alreadySeen := seen[stage]; alreadySeen {
			continue
		}
		if stage < last {
			return fmt.Errorf("movement packet %s arrived after a later movement stage", opcodeName(event.Opcode))
		}
		seen[stage] = struct{}{}
		last = stage
	}
	return nil
}

func opcodeName(opcode uint32) string {
	if name, ok := protocol.OpcodeNames[protocol.Opcode(opcode)]; ok {
		return name
	}
	return fmt.Sprintf("opcode 0x%X", opcode)
}

func exact(opcode protocol.Opcode) func(uint32) bool {
	return func(value uint32) bool { return value == uint32(opcode) }
}

func requireCreateBlock(event protocoltrace.Event, playerGUID uint64) error {
	found, err := inspectPlayerCreate(event, playerGUID, true)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("self player create block not found")
	}
	return nil
}

func containsPlayerCreate(event protocoltrace.Event, playerGUID uint64) (bool, error) {
	return inspectPlayerCreate(event, playerGUID, false)
}

func inspectPlayerCreate(event protocoltrace.Event, playerGUID uint64, strict bool) (bool, error) {
	payload, err := eventPayload(event)
	if err != nil {
		return false, err
	}
	if event.Opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		payload, err = protocol.DecompressUpdatePayload(payload)
		if err != nil {
			return false, fmt.Errorf("compressed player update decode failed: %w", err)
		}
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count == 0 {
		return false, fmt.Errorf("player update has no blocks")
	}
	playerFound := false
	for index := uint32(0); index < count; index++ {
		kind, err := reader.ReadU8()
		if err != nil {
			if strict || playerFound {
				return playerFound, fmt.Errorf("player update block %d is truncated: %w", index, err)
			}
			return false, nil
		}
		switch kind {
		case protocol.UpdateOutOfRangeObjects:
			outOfRange, err := reader.ReadU32()
			if err != nil {
				if strict || playerFound {
					return playerFound, fmt.Errorf("out-of-range update is truncated: %w", err)
				}
				return false, nil
			}
			for guidIndex := uint32(0); guidIndex < outOfRange; guidIndex++ {
				if _, err := reader.ReadPackedGUID(); err != nil {
					if strict || playerFound {
						return playerFound, fmt.Errorf("out-of-range GUID is truncated: %w", err)
					}
					return false, nil
				}
			}
		case protocol.UpdateCreateObject, protocol.UpdateCreateObject2:
			guid, typeID, _, err := parseCreateObjectBlock(reader, playerGUID)
			if err != nil {
				if guid == playerGUID && typeID == 4 || strict || playerFound {
					return playerFound || guid == playerGUID && typeID == 4, fmt.Errorf("create block %d: %w", index, err)
				}
				return false, nil
			}
			if (typeID == 1 || typeID == 2) && playerFound {
				return true, fmt.Errorf("item/container create block %d arrived after self-player create", index)
			}
			if typeID == 4 && guid == playerGUID {
				if playerFound {
					return true, fmt.Errorf("duplicate self-player create block %d", index)
				}
				playerFound = true
			}
		case protocol.UpdateValues:
			if err := skipValuesUpdate(reader); err != nil {
				if strict || playerFound {
					return playerFound, fmt.Errorf("values block %d: %w", index, err)
				}
				return false, nil
			}
		case protocol.UpdateMovement:
			if err := skipMovementUpdate(reader); err != nil {
				if strict || playerFound {
					return playerFound, fmt.Errorf("movement block %d: %w", index, err)
				}
				return false, nil
			}
		default:
			if strict || playerFound {
				return playerFound, fmt.Errorf("unsupported update block kind=%d", kind)
			}
			return false, nil
		}
	}
	if strict && !playerFound {
		return false, fmt.Errorf("self player create block not found")
	}
	return playerFound, nil
}

func parseCreateObjectBlock(reader *protocol.Buffer, playerGUID uint64) (uint64, uint8, map[int]uint32, error) {
	guid, err := reader.ReadPackedGUID()
	if err != nil {
		return 0, 0, nil, fmt.Errorf("create GUID is truncated: %w", err)
	}
	typeID, err := reader.ReadU8()
	if err != nil {
		return guid, 0, nil, fmt.Errorf("create type is truncated: %w", err)
	}
	flags, err := reader.ReadU16()
	if err != nil {
		return guid, typeID, nil, fmt.Errorf("create movement flags are truncated: %w", err)
	}
	if err := skipCreateMovement(reader, flags); err != nil {
		return guid, typeID, nil, err
	}
	mask, values, err := readUpdateValues(reader)
	if err != nil {
		return guid, typeID, nil, err
	}
	if typeID != 4 || guid != playerGUID {
		return guid, typeID, values, nil
	}
	if flags != 0x0061 && flags != 0x0065 {
		return guid, typeID, nil, fmt.Errorf("player create flags=0x%X, want 0x61 or 0x65", flags)
	}
	if len(mask) != 42 {
		return guid, typeID, nil, fmt.Errorf("player update mask blocks=%d, want 42", len(mask))
	}
	for field := 1326; field < len(mask)*32; field++ {
		if updateMaskHas(mask, field) {
			return guid, typeID, nil, fmt.Errorf("player update mask sets out-of-range field %d", field)
		}
	}
	for _, field := range []int{0, 2, 4, 23, 24, 32, 54, 59, 67, 68} {
		if !updateMaskHas(mask, field) {
			return guid, typeID, nil, fmt.Errorf("player update mask omits required field %d", field)
		}
	}
	if values[2] != 0x19 || values[23] == 0 || values[54] == 0 || values[67] == 0 || values[68] == 0 || values[24] == 0 || values[32] == 0 {
		return guid, typeID, nil, fmt.Errorf("player create required field values are invalid")
	}
	if values[59]&0x00000008 == 0 {
		return guid, typeID, nil, fmt.Errorf("player create omits UNIT_FLAG_PLAYER_CONTROLLED")
	}
	return guid, typeID, values, nil
}

func skipCreateMovement(reader *protocol.Buffer, flags uint16) error {
	if flags&0x20 != 0 {
		if err := skipLivingMovement(reader); err != nil {
			return err
		}
	} else if flags&0x100 != 0 {
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("position transport GUID is truncated: %w", err)
		}
		if _, err := reader.Read(32); err != nil {
			return fmt.Errorf("position movement is truncated: %w", err)
		}
	} else if flags&0x40 != 0 {
		if _, err := reader.Read(16); err != nil {
			return fmt.Errorf("stationary movement is truncated: %w", err)
		}
	}
	if flags&0x8 != 0 {
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("unknown movement field is truncated: %w", err)
		}
	}
	if flags&0x10 != 0 {
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("low-guid movement field is truncated: %w", err)
		}
	}
	if flags&0x4 != 0 {
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("target movement field is truncated: %w", err)
		}
	}
	if flags&0x2 != 0 {
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("transport movement field is truncated: %w", err)
		}
	}
	if flags&0x80 != 0 {
		if _, err := reader.Read(8); err != nil {
			return fmt.Errorf("vehicle movement field is truncated: %w", err)
		}
	}
	if flags&0x200 != 0 {
		if _, err := reader.Read(8); err != nil {
			return fmt.Errorf("rotation movement field is truncated: %w", err)
		}
	}
	return nil
}

func readUpdateValues(reader *protocol.Buffer) ([]uint32, map[int]uint32, error) {
	maskBlocks, err := reader.ReadU8()
	if err != nil {
		return nil, nil, fmt.Errorf("update mask is truncated: %w", err)
	}
	mask := make([]uint32, maskBlocks)
	for index := range mask {
		mask[index], err = reader.ReadU32()
		if err != nil {
			return nil, nil, fmt.Errorf("update mask block %d is truncated: %w", index, err)
		}
	}
	values := make(map[int]uint32)
	for index, bits := range mask {
		for bit := uint(0); bit < 32; bit++ {
			field := index*32 + int(bit)
			if bits&(uint32(1)<<bit) == 0 {
				continue
			}
			value, readErr := reader.ReadU32()
			if readErr != nil {
				return nil, nil, fmt.Errorf("update field %d is truncated: %w", field, readErr)
			}
			values[field] = value
		}
	}
	return mask, values, nil
}

func skipValuesUpdate(reader *protocol.Buffer) error {
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("values GUID is truncated: %w", err)
	}
	_, _, err := readUpdateValues(reader)
	return err
}

func skipMovementUpdate(reader *protocol.Buffer) error {
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("movement GUID is truncated: %w", err)
	}
	flags, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("movement flags are truncated: %w", err)
	}
	return skipCreateMovement(reader, flags)
}

func skipLivingMovement(reader *protocol.Buffer) error {
	movementFlags, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("player movement flags are truncated: %w", err)
	}
	extraFlags, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("player extra movement flags are truncated: %w", err)
	}
	if _, err := reader.Read(20); err != nil {
		return fmt.Errorf("player movement position is truncated: %w", err)
	}
	if movementFlags&0x200 != 0 {
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("player transport GUID is truncated: %w", err)
		}
		if _, err := reader.Read(21); err != nil {
			return fmt.Errorf("player transport offsets are truncated: %w", err)
		}
		if extraFlags&0x1 != 0 {
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("player interpolated transport time is truncated: %w", err)
			}
		}
	}
	if movementFlags&(0x2000|0x4000) != 0 || extraFlags&0x2 != 0 {
		if _, err := reader.ReadF32(); err != nil {
			return fmt.Errorf("player pitch is truncated: %w", err)
		}
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("player fall time is truncated: %w", err)
	}
	if movementFlags&0x1000 != 0 {
		if _, err := reader.Read(16); err != nil {
			return fmt.Errorf("player jump movement is truncated: %w", err)
		}
	}
	if movementFlags&0x04000000 != 0 {
		if _, err := reader.ReadF32(); err != nil {
			return fmt.Errorf("player spline elevation is truncated: %w", err)
		}
	}
	if _, err := reader.Read(36); err != nil {
		return fmt.Errorf("player movement speeds are truncated: %w", err)
	}
	if movementFlags&0x08000000 != 0 {
		if err := skipCreateSpline(reader); err != nil {
			return fmt.Errorf("player create spline is truncated: %w", err)
		}
	}
	return nil
}

func skipCreateSpline(reader *protocol.Buffer) error {
	flags, err := reader.ReadU32()
	if err != nil {
		return err
	}
	if flags&0x00020000 != 0 {
		if _, err := reader.ReadF32(); err != nil {
			return err
		}
	} else if flags&0x00010000 != 0 {
		if _, err := reader.ReadPackedGUID(); err != nil {
			return err
		}
	} else if flags&0x00008000 != 0 {
		if _, err := reader.Read(12); err != nil {
			return err
		}
	}
	if _, err := reader.Read(28); err != nil {
		return err
	}
	nodes, err := reader.ReadU32()
	if err != nil {
		return err
	}
	if uint64(nodes)*12 > uint64(reader.Remaining()) {
		return fmt.Errorf("spline node count %d exceeds remaining bytes", nodes)
	}
	if _, err := reader.Read(int(nodes) * 12); err != nil {
		return err
	}
	if _, err := reader.ReadU8(); err != nil {
		return err
	}
	if _, err := reader.Read(12); err != nil {
		return err
	}
	return nil
}

func checkCreateMovementParser() error {
	for _, test := range []struct {
		transport   bool
		spline      bool
		splineFlags uint32
	}{{false, false, 0}, {true, false, 0}, {false, true, 0x00008000}, {true, true, 0}} {
		payload := protocol.NewBuffer(256)
		var movementFlags uint32
		if test.transport {
			movementFlags |= 0x200
		}
		if test.spline {
			movementFlags |= 0x08000000
		}
		extraFlags := uint16(0)
		if test.transport {
			extraFlags = 1
		}
		payload.WriteU32(movementFlags)
		payload.WriteU16(extraFlags)
		payload.WriteU32(1)
		for i := 0; i < 4; i++ {
			payload.WriteF32(0)
		}
		if test.transport {
			payload.WritePackedGUID(0x2000000000000001)
			for i := 0; i < 4; i++ {
				payload.WriteF32(0)
			}
			payload.WriteU32(2)
			payload.WriteI8(0)
			payload.WriteU32(3)
		}
		payload.WriteU32(0)
		for i := 0; i < 9; i++ {
			payload.WriteF32(1)
		}
		if test.spline {
			payload.WriteU32(test.splineFlags)
			if test.splineFlags&0x00020000 != 0 {
				payload.WriteF32(0)
			} else if test.splineFlags&0x00010000 != 0 {
				payload.WritePackedGUID(0x106)
			} else if test.splineFlags&0x00008000 != 0 {
				payload.WriteF32(0)
				payload.WriteF32(0)
				payload.WriteF32(0)
			}
			payload.WriteU32(1)
			payload.WriteU32(2)
			payload.WriteU32(3)
			payload.WriteF32(1)
			payload.WriteF32(1)
			payload.WriteF32(0)
			payload.WriteU32(0)
			payload.WriteU32(2)
			for i := 0; i < 6; i++ {
				payload.WriteF32(0)
			}
			payload.WriteU8(0)
			for i := 0; i < 3; i++ {
				payload.WriteF32(0)
			}
		}
		reader := protocol.NewReader(payload.Bytes())
		if err := skipLivingMovement(reader); err != nil || reader.Remaining() != 0 {
			return fmt.Errorf("create movement transport=%t spline=%t remaining=%d: %v", test.transport, test.spline, reader.Remaining(), err)
		}
	}
	return nil
}

func updateMaskHas(mask []uint32, field int) bool {
	block := field / 32
	bit := uint(field % 32)
	return block >= 0 && block < len(mask) && mask[block]&(uint32(1)<<bit) != 0
}

func eventPayload(event protocoltrace.Event) ([]byte, error) {
	return protocoltrace.Trace{Events: []protocoltrace.Event{event}}.Payload(event)
}

func runRealCharacterLoginReplay(workDir string, guid uint64, tracePath string, petCooldownSpell, petPowerSpell, petXPAward uint32) error {
	workDir, err := filepath.Abs(workDir)
	if err != nil {
		return err
	}
	for _, name := range []string{"auth.db", "characters.db", "world.db"} {
		if _, err := os.Stat(filepath.Join(workDir, name)); err != nil {
			return fmt.Errorf("replay work directory is missing %s: %w", name, err)
		}
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg := config.Default()
	cfg.Backend = string(database.BackendSQLite)
	cfg.LuaEnabled = false
	cfg.DataDir = workDir
	cfg.AuthDatabaseFile, cfg.CharactersDatabaseFile, cfg.WorldDatabaseFile = filepath.Join(workDir, "auth.db"), filepath.Join(workDir, "characters.db"), filepath.Join(workDir, "world.db")
	cfg.SchemaDir, cfg.GameDataDir = filepath.Join(root, "sql"), filepath.Join(root, "data")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stores, err := database.OpenSet(ctx, cfg)
	if err != nil {
		return err
	}
	defer stores.Close()
	if guid == 0 {
		var selectedGUID int64
		if err := stores.Characters.DB.QueryRowContext(ctx, "SELECT guid FROM characters ORDER BY guid LIMIT 1").Scan(&selectedGUID); err != nil || selectedGUID <= 0 {
			var characterCount int64
			_ = stores.Characters.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE guid > 0").Scan(&characterCount)
			return fmt.Errorf("replay database contains no usable real character rows (count=%d)", characterCount)
		}
		guid = uint64(selectedGUID)
	}
	server := world.NewServer(stores, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg.RealmID, cfg)
	if err := server.Initialize(ctx); err != nil {
		server.Stop()
		return err
	}
	before, err := snapshotCharacterState(stores.Characters.DB, guid)
	if err != nil {
		server.Stop()
		return err
	}
	petBefore, err := snapshotActivePetLoginState(ctx, stores.Characters.DB, guid)
	if err != nil {
		server.Stop()
		return err
	}
	var trace protocoltrace.Trace
	var replayErr error
	if petCooldownSpell != 0 {
		trace, replayErr = world.ReplayCharacterPetCooldown(ctx, server, guid, petCooldownSpell)
	} else if petPowerSpell != 0 {
		trace, replayErr = world.ReplayCharacterPetPower(ctx, server, guid, petPowerSpell)
	} else if petXPAward != 0 {
		trace, replayErr = world.ReplayCharacterPetXP(ctx, server, guid, petXPAward)
	} else {
		trace, replayErr = world.ReplayCharacterLogin(ctx, server, guid)
	}
	cancel()
	server.Stop()
	after, snapshotErr := snapshotCharacterState(stores.Characters.DB, guid)
	if tracePath == "" {
		tracePath = filepath.Join(workDir, "login-replay.jsonl")
	} else if !filepath.IsAbs(tracePath) {
		tracePath = filepath.Join(root, tracePath)
	}
	file, err := os.Create(tracePath)
	if err != nil {
		return err
	}
	writeErr := trace.Write(file)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if replayErr != nil {
		return fmt.Errorf("real-character login replay failed (trace saved): %w", replayErr)
	}
	if snapshotErr != nil {
		return snapshotErr
	}
	if err := validateCharacterStateDelta(before, after); err != nil {
		return fmt.Errorf("real-character state delta mismatch (trace saved): %w", err)
	}
	beforeCharacter, beforeFound := before["characters"]
	afterCharacter, afterFound := after["characters"]
	if !beforeFound || !afterFound || beforeCharacter.Rows != 1 || afterCharacter.Rows != 1 {
		return fmt.Errorf("real-character login replay changed character-row presence before=%t after=%t", beforeFound && beforeCharacter.Rows == 1, afterFound && afterCharacter.Rows == 1)
	}
	if err := checkLogin(trace, 0); err != nil {
		return fmt.Errorf("real-character login packet replay failed: %w", err)
	}
	if err := checkPetLoginState(trace, petBefore); err != nil {
		return fmt.Errorf("real-character pet state packet replay failed: %w", err)
	}
	changed := make([]string, 0)
	for table, beforeSnapshot := range before {
		afterSnapshot, exists := after[table]
		if !exists {
			changed = append(changed, table+"(missing)")
			continue
		}
		if afterSnapshot.Digest != beforeSnapshot.Digest {
			changed = append(changed, fmt.Sprintf("%s(rows=%d->%d;fields=%s)", table, beforeSnapshot.Rows, afterSnapshot.Rows, strings.Join(changedCharacterColumns(beforeSnapshot, afterSnapshot), "+")))
		}
	}
	for table := range after {
		if _, existed := before[table]; !existed {
			changed = append(changed, table+"(new)")
		}
	}
	sort.Strings(changed)
	if petCooldownSpell != 0 {
		fmt.Printf("real-character pet cooldown replay passed spell=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", petCooldownSpell, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	} else if petPowerSpell != 0 {
		fmt.Printf("real-character pet power replay passed spell=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", petPowerSpell, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	} else if petXPAward != 0 {
		fmt.Printf("real-character pet XP replay passed award=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", petXPAward, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	} else {
		fmt.Printf("real-character login replay passed lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	}
	return nil
}

type replayPetLoginState struct {
	petType    uint32
	mana       uint32
	happiness  uint32
	experience uint32
}

func snapshotActivePetLoginState(ctx context.Context, db *sql.DB, ownerGUID uint64) (*replayPetLoginState, error) {
	var id, petType, mana, happiness, experience int64
	err := db.QueryRowContext(ctx, "SELECT id, COALESCE(PetType, 0), COALESCE(curmana, 0), COALESCE(curhappiness, 0), COALESCE(exp, 0) FROM character_pet WHERE owner = ? AND slot = 0 LIMIT 1", ownerGUID).Scan(&id, &petType, &mana, &happiness, &experience)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	values := []int64{petType, mana, happiness, experience}
	converted := make([]uint32, len(values))
	for index, value := range values {
		if value < 0 || value > int64(^uint32(0)) {
			return nil, fmt.Errorf("active pet state field %d is outside uint32 range", index)
		}
		converted[index] = uint32(value)
	}
	return &replayPetLoginState{petType: converted[0], mana: converted[1], happiness: converted[2], experience: converted[3]}, nil
}

func checkPetLoginState(trace protocoltrace.Trace, saved *replayPetLoginState) error {
	if saved == nil {
		return nil
	}
	petGUID := uint64(0)
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_PET_SPELLS) {
			continue
		}
		payload, err := eventPayload(event)
		if err != nil {
			return err
		}
		reader := protocol.NewReader(payload)
		petGUID, err = reader.ReadU64()
		if err != nil {
			return fmt.Errorf("SMSG_PET_SPELLS pet GUID: %w", err)
		}
		if petGUID != 0 {
			break
		}
	}
	if petGUID == 0 {
		return nil
	}
	fields, found, err := petCreateFields(trace, petGUID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("active pet %X has no create update", petGUID)
	}
	maxMana := fields[33]
	expectedMana := saved.mana
	if expectedMana > maxMana {
		expectedMana = maxMana
	}
	if fields[25] != expectedMana || fields[77] != saved.experience {
		return fmt.Errorf("active pet power/experience fields mismatch mana=%d/%d experience=%d/%d", fields[25], expectedMana, fields[77], saved.experience)
	}
	if saved.petType == 1 {
		expectedHappiness := saved.happiness
		if expectedHappiness > 1050000 {
			expectedHappiness = 1050000
		}
		if fields[23]>>24&0xFF != 2 || fields[27] != 100 || fields[35] != 100 || fields[29] != expectedHappiness || fields[37] != 1050000 {
			return fmt.Errorf("hunter pet power fields mismatch type=%d focus=%d/%d happiness=%d/%d", fields[23]>>24&0xFF, fields[27], fields[35], fields[29], expectedHappiness)
		}
	}
	return nil
}

func petCreateFields(trace protocoltrace.Trace, petGUID uint64) (map[int]uint32, bool, error) {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) && event.Opcode != uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
			continue
		}
		payload, err := eventPayload(event)
		if err != nil {
			return nil, false, err
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
			payload, err = protocol.DecompressUpdatePayload(payload)
			if err != nil {
				return nil, false, fmt.Errorf("compressed pet update decode failed: %w", err)
			}
		}
		reader := protocol.NewReader(payload)
		count, err := reader.ReadU32()
		if err != nil {
			return nil, false, fmt.Errorf("pet update block count: %w", err)
		}
		for index := uint32(0); index < count; index++ {
			kind, err := reader.ReadU8()
			if err != nil {
				return nil, false, fmt.Errorf("pet update block %d: %w", index, err)
			}
			switch kind {
			case protocol.UpdateOutOfRangeObjects:
				outOfRange, err := reader.ReadU32()
				if err != nil {
					return nil, false, err
				}
				for guidIndex := uint32(0); guidIndex < outOfRange; guidIndex++ {
					if _, err := reader.ReadPackedGUID(); err != nil {
						return nil, false, err
					}
				}
			case protocol.UpdateCreateObject, protocol.UpdateCreateObject2:
				guid, typeID, fields, err := parseCreateObjectBlock(reader, 0)
				if err != nil {
					return nil, false, err
				}
				if guid == petGUID {
					if typeID != 3 {
						return nil, false, fmt.Errorf("active pet create type=%d, want unit type 3", typeID)
					}
					return fields, true, nil
				}
			case protocol.UpdateValues:
				if err := skipValuesUpdate(reader); err != nil {
					return nil, false, err
				}
			case protocol.UpdateMovement:
				if err := skipMovementUpdate(reader); err != nil {
					return nil, false, err
				}
			default:
				return nil, false, fmt.Errorf("unsupported pet update block kind=%d", kind)
			}
		}
	}
	return nil, false, nil
}

type characterTableSnapshot struct {
	Rows    int
	Digest  string
	Columns map[string]string
}

func changedCharacterColumns(before, after characterTableSnapshot) []string {
	changed := make([]string, 0)
	for column, digest := range before.Columns {
		if after.Columns[column] != digest {
			changed = append(changed, column)
		}
	}
	for column := range after.Columns {
		if _, exists := before.Columns[column]; !exists {
			changed = append(changed, column)
		}
	}
	sort.Strings(changed)
	return changed
}

func validateCharacterStateDelta(before, after map[string]characterTableSnapshot) error {
	allowedColumns := map[string]map[string]struct{}{
		"characters":     {"exploredZones": {}, "orientation": {}, "position_x": {}, "position_y": {}, "position_z": {}},
		"character_aura": {"remainTime": {}},
		"character_pet":  {"curhealth": {}, "curmana": {}, "exp": {}, "level": {}, "savetime": {}},
		"pet_spell":      {"active": {}, "guid": {}, "spell": {}},
	}
	allowedRowChanges := map[string]bool{"character_aura": true, "pet_spell": true}
	for table, beforeTable := range before {
		afterTable, exists := after[table]
		if !exists {
			return fmt.Errorf("snapshot table %s disappeared", table)
		}
		changedColumns := changedCharacterColumns(beforeTable, afterTable)
		rowsChanged := beforeTable.Rows != afterTable.Rows
		digestChanged := beforeTable.Digest != afterTable.Digest
		if len(changedColumns) == 0 && !rowsChanged && !digestChanged {
			continue
		}
		if len(changedColumns) == 0 && !rowsChanged && digestChanged {
			return fmt.Errorf("unclassified row composition change table=%s", table)
		}
		allowed, exists := allowedColumns[table]
		if !exists || rowsChanged && !allowedRowChanges[table] {
			return fmt.Errorf("unclassified state mutation table=%s rows=%d->%d columns=%s", table, beforeTable.Rows, afterTable.Rows, strings.Join(changedColumns, "+"))
		}
		for _, column := range changedColumns {
			if _, exists := allowed[column]; !exists {
				return fmt.Errorf("unclassified state mutation table=%s column=%s", table, column)
			}
		}
	}
	for table := range after {
		if _, exists := before[table]; !exists {
			return fmt.Errorf("snapshot table %s appeared", table)
		}
	}
	return nil
}

func snapshotCharacterState(db *sql.DB, guid uint64) (map[string]characterTableSnapshot, error) {
	queries := []struct{ name, sql string }{
		{"characters", "SELECT * FROM characters WHERE guid = ?"},
		{"character_account_data", "SELECT * FROM character_account_data WHERE guid = ?"},
		{"account_data", "SELECT * FROM account_data WHERE accountId = (SELECT account FROM characters WHERE guid = ?)"},
		{"account_instance_times", "SELECT * FROM account_instance_times WHERE accountId = (SELECT account FROM characters WHERE guid = ?)"},
		{"account_tutorial", "SELECT * FROM account_tutorial WHERE accountId = (SELECT account FROM characters WHERE guid = ?)"},
		{"character_achievement", "SELECT * FROM character_achievement WHERE guid = ?"},
		{"character_achievement_progress", "SELECT * FROM character_achievement_progress WHERE guid = ?"},
		{"character_action", "SELECT * FROM character_action WHERE guid = ?"},
		{"character_arena_stats", "SELECT * FROM character_arena_stats WHERE guid = ?"},
		{"arena_team_member", "SELECT * FROM arena_team_member WHERE guid = ?"},
		{"character_banned", "SELECT * FROM character_banned WHERE guid = ?"},
		{"character_battleground_random", "SELECT * FROM character_battleground_random WHERE guid = ?"},
		{"battleground_deserters", "SELECT * FROM battleground_deserters WHERE guid = ?"},
		{"character_declinedname", "SELECT * FROM character_declinedname WHERE guid = ?"},
		{"character_fishingsteps", "SELECT * FROM character_fishingsteps WHERE guid = ?"},
		{"character_gifts", "SELECT * FROM character_gifts WHERE guid = ?"},
		{"character_inventory", "SELECT * FROM character_inventory WHERE guid = ?"},
		{"inventory_item_instances", "SELECT ii.* FROM item_instance ii JOIN character_inventory ci ON ci.item = ii.guid WHERE ci.guid = ?"},
		{"item_refund_instance", "SELECT ri.* FROM item_refund_instance ri JOIN character_inventory ci ON ci.item = ri.item_guid WHERE ci.guid = ?"},
		{"item_soulbound_trade_data", "SELECT st.* FROM item_soulbound_trade_data st JOIN character_inventory ci ON ci.item = st.itemGuid WHERE ci.guid = ?"},
		{"character_spell", "SELECT * FROM character_spell WHERE guid = ?"},
		{"character_spell_cooldown", "SELECT * FROM character_spell_cooldown WHERE guid = ?"},
		{"character_skills", "SELECT * FROM character_skills WHERE guid = ?"},
		{"character_reputation", "SELECT * FROM character_reputation WHERE guid = ?"},
		{"character_queststatus", "SELECT * FROM character_queststatus WHERE guid = ?"},
		{"character_queststatus_rewarded", "SELECT * FROM character_queststatus_rewarded WHERE guid = ?"},
		{"character_queststatus_daily", "SELECT * FROM character_queststatus_daily WHERE guid = ?"},
		{"character_queststatus_weekly", "SELECT * FROM character_queststatus_weekly WHERE guid = ?"},
		{"character_queststatus_monthly", "SELECT * FROM character_queststatus_monthly WHERE guid = ?"},
		{"character_queststatus_seasonal", "SELECT * FROM character_queststatus_seasonal WHERE guid = ?"},
		{"character_aura", "SELECT * FROM character_aura WHERE guid = ?"},
		{"character_talent", "SELECT * FROM character_talent WHERE guid = ?"},
		{"character_glyphs", "SELECT * FROM character_glyphs WHERE guid = ?"},
		{"character_homebind", "SELECT * FROM character_homebind WHERE guid = ?"},
		{"character_instance", "SELECT * FROM character_instance WHERE guid = ?"},
		{"character_battleground_data", "SELECT * FROM character_battleground_data WHERE guid = ?"},
		{"character_equipmentsets", "SELECT * FROM character_equipmentsets WHERE guid = ?"},
		{"character_pet", "SELECT * FROM character_pet WHERE owner = ?"},
		{"character_pet_declinedname", "SELECT * FROM character_pet_declinedname WHERE owner = ?"},
		{"pet_aura", "SELECT * FROM pet_aura WHERE guid IN (SELECT id FROM character_pet WHERE owner = ?)"},
		{"pet_spell", "SELECT * FROM pet_spell WHERE guid IN (SELECT id FROM character_pet WHERE owner = ?)"},
		{"pet_spell_cooldown", "SELECT * FROM pet_spell_cooldown WHERE guid IN (SELECT id FROM character_pet WHERE owner = ?)"},
		{"character_social", "SELECT * FROM character_social WHERE guid = ?"},
		{"character_stats", "SELECT * FROM character_stats WHERE guid = ?"},
		{"characters_npcbot", "SELECT * FROM characters_npcbot WHERE owner = ?"},
		{"corpse", "SELECT * FROM corpse WHERE guid = ?"},
		{"petition", "SELECT * FROM petition WHERE ownerguid = ?"},
		{"petition_sign", "SELECT * FROM petition_sign WHERE ? IN (ownerguid, playerguid)"},
		{"guild_member", "SELECT * FROM guild_member WHERE guid = ?"},
		{"guild_member_withdraw", "SELECT * FROM guild_member_withdraw WHERE guid = ?"},
		{"guild_bank_eventlog_player_rows", "SELECT * FROM guild_bank_eventlog WHERE PlayerGuid = ?"},
		{"guild_eventlog_player_rows", "SELECT * FROM guild_eventlog WHERE ? IN (PlayerGuid1, PlayerGuid2)"},
		{"group_member", "SELECT * FROM group_member WHERE memberGuid = ?"},
		{"received_mail", "SELECT * FROM mail WHERE receiver = ?"},
		{"sent_mail", "SELECT * FROM mail WHERE sender = ?"},
		{"received_mail_items", "SELECT mi.* FROM mail_items mi JOIN mail m ON m.id = mi.mail_id WHERE m.receiver = ?"},
		{"received_mail_item_instances", "SELECT ii.* FROM item_instance ii JOIN mail_items mi ON mi.item_guid = ii.guid WHERE mi.receiver = ?"},
		{"auctionhouse_player_rows", "SELECT * FROM auctionhouse WHERE ? IN (itemowner, buyguid)"},
		{"auctionbidders_player_rows", "SELECT * FROM auctionbidders WHERE bidderguid = ?"},
		{"auction_item_instances", "SELECT ii.* FROM item_instance ii JOIN auctionhouse ah ON ah.itemguid = ii.guid WHERE ? IN (ah.itemowner, ah.buyguid)"},
		{"calendar_events_created", "SELECT * FROM calendar_events WHERE creator = ?"},
		{"calendar_invites_player_rows", "SELECT * FROM calendar_invites WHERE ? IN (invitee, sender)"},
		{"gm_ticket_player_rows", "SELECT * FROM gm_ticket WHERE playerGuid = ?"},
		{"gm_survey_player_rows", "SELECT * FROM gm_survey WHERE guid = ?"},
		{"gm_subsurvey_player_rows", "SELECT gs.* FROM gm_subsurvey gs JOIN gm_survey g ON g.surveyId = gs.surveyId WHERE g.guid = ?"},
		{"lag_reports", "SELECT * FROM lag_reports WHERE guid = ?"},
		{"pvpstats_players", "SELECT * FROM pvpstats_players WHERE character_guid = ?"},
		{"quest_tracker", "SELECT * FROM quest_tracker WHERE character_guid = ?"},
	}
	result := make(map[string]characterTableSnapshot, len(queries))
	for _, query := range queries {
		rows, err := db.Query(query.sql, guid)
		if err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", query.name, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return nil, err
		}
		rowHashes := make([]string, 0)
		columnValues := make([][]string, len(columns))
		for rows.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for index := range values {
				targets[index] = &values[index]
			}
			if err := rows.Scan(targets...); err != nil {
				rows.Close()
				return nil, err
			}
			var encoded strings.Builder
			for index, value := range values {
				var columnValue strings.Builder
				if data, ok := value.([]byte); ok {
					fmt.Fprintf(&columnValue, "bytes:%x", data)
				} else {
					fmt.Fprintf(&columnValue, "%T:%v", value, value)
				}
				columnValues[index] = append(columnValues[index], columnValue.String())
				fmt.Fprintf(&encoded, "%d:%s\x00", columnValue.Len(), columnValue.String())
			}
			hash := sha256.Sum256([]byte(encoded.String()))
			rowHashes = append(rowHashes, hex.EncodeToString(hash[:]))
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		sort.Strings(rowHashes)
		digest := sha256.Sum256([]byte(strings.Join(rowHashes, "\n")))
		columnDigests := make(map[string]string, len(columns))
		for index, column := range columns {
			sort.Strings(columnValues[index])
			columnDigest := sha256.Sum256([]byte(strings.Join(columnValues[index], "\n")))
			columnDigests[column] = hex.EncodeToString(columnDigest[:])
		}
		result[query.name] = characterTableSnapshot{Rows: len(rowHashes), Digest: hex.EncodeToString(digest[:]), Columns: columnDigests}
	}
	return result, nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
