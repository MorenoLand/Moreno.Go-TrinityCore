package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

var sourcePlayerCreateVisibility = [...]uint32{
	0xFFFFFFDF, 0xFFFFFFFF, 0xFFFB1FFF, 0xFFFFFFFF, 0xBFF7FFFF, 0xEF7BDEF7, 0x7BDEF7BD, 0xDEF7BDEF,
	0xFFBDEF7B, 0xFFFFFFFF, 0xFFFFFFF7, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF,
	0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF,
	0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF,
	0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF,
	0xFFFFFFFF, 0x00003FFF,
}

func main() {
	tracePath := flag.String("trace", "", "recorded protocol trace JSONL")
	selfCheck := flag.Bool("self-check", false, "validate the login loading-order regression guard")
	replayWork := flag.String("replay-work", "", "isolated work directory containing auth.db, characters.db, and world.db; runs core login with Eluna disabled")
	replayGUID := flag.Uint64("replay-guid", 0, "character GUID for an in-process login replay; 0 selects the first real character")
	replayPeerGUID := flag.Uint64("replay-peer-guid", 0, "second character GUID for a same-server two-session login replay")
	replayPetCooldownSpell := flag.Uint("replay-pet-cooldown-spell", 0, "after login, assert a saved pet category cooldown rejects this spell")
	replayPetPowerSpell := flag.Uint("replay-pet-power-spell", 0, "after login, verify a known pet spell spends and reports its DBC power cost")
	replayPetXPAward := flag.Uint("replay-pet-xp", 0, "after login, apply a hunter-pet XP award and verify fields and persistence")
	replayPetAuraSourceSpell := flag.Uint("replay-pet-aura-source-spell", 0, "inject an owner source spell into the isolated character copy and verify its mapped pet aura")
	replayPetFocusAuraSpell := flag.Uint("replay-pet-focus-aura-spell", 0, "replay a DBC Focus regeneration aura through the pet timer and power packet")
	replayPetFeedSpell := flag.Uint("replay-pet-feed-spell", 0, "after login, cast a pet-feed spell against the supplied inventory item")
	replayPetFeedItem := flag.Uint64("replay-pet-feed-item", 0, "inventory item GUID used by the pet-feed replay")
	replayPetCritter := flag.Bool("replay-pet-critter", false, "verify a saved critter loads without controlled-pet fields, spells, or talents")
	replayLFGDungeon := flag.Uint("replay-lfg-dungeon", 0, "after login, teleport through the LFG entrance and verify saved battleground return data")
	replayFarTeleport := flag.Bool("replay-far-teleport", false, "replay a cross-map transfer and WORLDPORT_ACK from a valid world spawn")
	replayInstanceMap := flag.Uint("replay-instance-map", 0, "dungeon map used to exercise the source account instance-entry timer")
	replayInstanceID := flag.Uint("replay-instance-id", 0, "instance ID used with --replay-instance-map")
	replayStatsMinLevel := flag.Uint("replay-stats-min-level", 0, "save and verify source character_stats output for characters at or above this level")
	replayPlayerStartMessage := flag.Bool("replay-player-start-message", false, "verify the source PlayerStart.String first-login packet order")
	replayQuestRewardTwiceID := flag.Uint("replay-quest-reward-twice", 0, "replay a one-time quest reward followed by a duplicate turn-in")
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
		statsReplayRequested := *replayStatsMinLevel != 0
		petFeedRequested := *replayPetFeedSpell != 0 || *replayPetFeedItem != 0
		lfgReplayRequested := *replayLFGDungeon != 0
		farTeleportReplayRequested := *replayFarTeleport
		instanceReplayRequested := *replayInstanceMap != 0 || *replayInstanceID != 0
		startMessageRequested := *replayPlayerStartMessage
		questRewardReplayRequested := *replayQuestRewardTwiceID != 0
		petCritterReplayRequested := *replayPetCritter
		petReplayCount := 0
		for _, requested := range []bool{*replayPetCooldownSpell != 0, *replayPetPowerSpell != 0, *replayPetXPAward != 0, *replayPetAuraSourceSpell != 0, *replayPetFocusAuraSpell != 0, petFeedRequested} {
			if requested {
				petReplayCount++
			}
		}
		postLoginReplayCount := petReplayCount
		for _, requested := range []bool{lfgReplayRequested, farTeleportReplayRequested, instanceReplayRequested, statsReplayRequested, startMessageRequested, questRewardReplayRequested, petCritterReplayRequested} {
			if requested {
				postLoginReplayCount++
			}
		}
		if postLoginReplayCount > 1 || petFeedRequested && (*replayPetFeedSpell == 0 || *replayPetFeedItem == 0) || instanceReplayRequested && (*replayInstanceMap == 0 || *replayInstanceID == 0) {
			fail("choose only one post-login replay scenario")
		}
		if *replayPeerGUID != 0 && postLoginReplayCount != 0 {
			fail("paired login replay cannot be combined with a post-login replay scenario")
		}
		if err := runRealCharacterLoginReplay(*replayWork, *replayGUID, *replayPeerGUID, *replayTrace, uint32(*replayPetCooldownSpell), uint32(*replayPetPowerSpell), uint32(*replayPetXPAward), uint32(*replayPetAuraSourceSpell), uint32(*replayPetFocusAuraSpell), uint32(*replayPetFeedSpell), *replayPetFeedItem, uint32(*replayLFGDungeon), uint32(*replayInstanceMap), uint32(*replayInstanceID), uint32(*replayStatsMinLevel), startMessageRequested, uint32(*replayQuestRewardTwiceID), petCritterReplayRequested, farTeleportReplayRequested); err != nil {
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
	if err := checkPartyMemberStatsFullPacket(); err != nil {
		return err
	}
	if err := checkPlayerStatsConfig(); err != nil {
		return err
	}
	if err := checkAuraGUIDRepresentations(); err != nil {
		return err
	}
	if err := checkSpellPowerEnchantmentDBC(); err != nil {
		return err
	}
	if err := checkRandomSuffixSpellPowerDBC(); err != nil {
		return err
	}
	statEnchant := wotlk.SpellItemEnchantmentEntry{Effects: [3]uint32{5, 5, 5}, EffectPointsMin: [3]uint32{20, 30, 40}, EffectArg: [3]uint32{0, 1, 45}}
	if world.ResolveEquippedItemStatEnchant(statEnchant, 0, 80, 0, false) != 20 || world.ResolveEquippedItemStatEnchant(statEnchant, 1, 80, 0, false) != 30 || world.ResolveEquippedItemStatEnchant(statEnchant, 45, 80, 0, false) != 40 {
		return fmt.Errorf("source item enchant stat args did not map mana, health, and spell power")
	}
	suffixStatEnchant := statEnchant
	suffixStatEnchant.EffectPointsMin = [3]uint32{}
	if world.ResolveRandomSuffixItemStatEnchant(suffixStatEnchant, 0, 80, 0, false, 17) != 17 || world.ResolveRandomSuffixItemStatEnchant(suffixStatEnchant, 1, 80, 0, false, 17) != 17 || world.ResolveRandomSuffixItemStatEnchant(suffixStatEnchant, 45, 80, 0, false, 17) != 17 {
		return fmt.Errorf("source random-suffix stat args did not map mana, health, and spell power")
	}
	manaStat, healthStat, strengthStat, hitStat := world.ResolvePlayerItemStatBonus(0, 20), world.ResolvePlayerItemStatBonus(1, 30), world.ResolvePlayerItemStatBonus(4, 5), world.ResolvePlayerItemStatBonus(31, 7)
	if manaStat.Mana != 20 || healthStat.Health != 30 || strengthStat.Stats[0] != 5 || hitStat.CombatRatings[5] != 7 || hitStat.CombatRatings[6] != 7 || hitStat.CombatRatings[7] != 7 {
		return fmt.Errorf("source item-template stat IDs did not map to player resource/stat/rating state")
	}
	prismatic := wotlk.SpellItemEnchantmentEntry{RequiredSkillID: 164, RequiredSkillRank: 50}
	for _, test := range []struct {
		color   uint32
		entry   wotlk.SpellItemEnchantmentEntry
		found   bool
		skill   uint32
		allowed bool
	}{{1, wotlk.SpellItemEnchantmentEntry{}, false, 0, true}, {0, wotlk.SpellItemEnchantmentEntry{}, false, 0, false}, {0, prismatic, true, 49, false}, {0, prismatic, true, 50, true}} {
		if actual, expected := world.ResolveGemSocketEnchantActive(test.color, test.entry, test.found, test.skill), sourceGemSocketEnchantActive(test.color, test.entry, test.found, test.skill); actual != test.allowed || expected != test.allowed {
			return fmt.Errorf("prismatic socket gate source=%t Go=%t, want %t", expected, actual, test.allowed)
		}
	}
	if world.PlayerCreateUpdateFlags(false, false) != 0x0060 || world.PlayerCreateUpdateFlags(true, false) != 0x0061 || world.PlayerCreateUpdateFlags(false, true) != 0x0064 {
		return fmt.Errorf("player create update flags do not match victim/self source flags")
	}
	if value := world.ResolvePlayerParryPercentage(1, false, 100, 100, 10, 10, 10); value != 0 {
		return fmt.Errorf("parry percentage ignored the source false-by-default capability gate: %f", value)
	}
	if value := world.ResolvePlayerParryPercentage(1, true, 100, 100, 0, 0, 0); math.Abs(float64(value-5)) > 0.0001 {
		return fmt.Errorf("base parry percentage produced %f, want 5", value)
	}
	if value := world.ResolvePlayerParryPercentage(1, true, 95, 100, 0, 0, 0); math.Abs(float64(value-4.8)) > 0.0001 {
		return fmt.Errorf("defense-skill parry adjustment produced %f, want 4.8", value)
	}
	if value := world.ResolvePlayerParryPercentage(5, true, 100, 100, 10, 10, 10); value != 0 {
		return fmt.Errorf("zero-cap source class produced parry percentage %f", value)
	}
	wantParry := float32(47.003525)*10.2/(10.2+float32(47.003525)*0.9560) + 7
	if value := world.ResolvePlayerParryPercentage(1, true, 100, 100, 10, 5, 2); math.Abs(float64(value-wantParry)) > 0.0001 {
		return fmt.Errorf("rated/aura parry formula produced %f, want %f", value, wantParry)
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
	preCreateCombat := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_ATTACK_START)}, playerCreateEvent}}
	if err := rejectWorldActivityBeforePlayerCreate(preCreateCombat, 0); err == nil {
		return fmt.Errorf("world combat packet before the self-player create was not rejected")
	}
	postCreateCombat := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_ATTACK_START)}}}
	if err := rejectWorldActivityBeforePlayerCreate(postCreateCombat, 0); err != nil {
		return fmt.Errorf("world combat packet after the self-player create was rejected: %w", err)
	}
	badEarlyLoginOrder := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_CONTACT_LIST)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY)}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_CONTACT_LIST)}, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}}}
	if _, err := findOrderedLoginStages(badEarlyLoginOrder, 0, loginOrderStages); err == nil {
		return fmt.Errorf("out-of-order pre-map login packet was not rejected")
	}
	badEarlyWorldStates := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY)}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_CONTACT_LIST)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}, playerCreateEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_INIT_WORLD_STATES)}}}
	if _, err := findOrderedLoginStages(badEarlyWorldStates, 0, loginOrderStages); err == nil {
		return fmt.Errorf("post-map world state sent before player create was not rejected")
	}
	movementAura := protocol.NewBuffer(8)
	movementAura.WritePackedGUID(loginGUID)
	validMovement := protocoltrace.Trace{Events: []protocoltrace.Event{loginEvent, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_FEATHER_FALL)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_HOVER)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_FORCE_MOVE_ROOT)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MULTIPLE_MOVES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL), Payload: base64.StdEncoding.EncodeToString(movementAura.Bytes())}}}
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
	if err := checkPetFoodRules(); err != nil {
		return fmt.Errorf("pet food rules check failed: %w", err)
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

func checkPartyMemberStatsFullPacket() error {
	guid := uint64(0xF130000000001234)
	flags := protocol.GroupUpdateFlagStatus | protocol.GroupUpdateFlagCurrentHealth | protocol.GroupUpdateFlagMaximumHealth | protocol.GroupUpdateFlagPowerType | protocol.GroupUpdateFlagCurrentPower | protocol.GroupUpdateFlagMaximumPower | protocol.GroupUpdateFlagLevel | protocol.GroupUpdateFlagZone | protocol.GroupUpdateFlagPosition | protocol.GroupUpdateFlagAuras | protocol.GroupUpdateFlagPetGUID | protocol.GroupUpdateFlagPetName | protocol.GroupUpdateFlagPetModelID | protocol.GroupUpdateFlagPetCurrentHealth | protocol.GroupUpdateFlagPetMaximumHealth | protocol.GroupUpdateFlagPetPowerType | protocol.GroupUpdateFlagPetCurrentPower | protocol.GroupUpdateFlagPetMaximumPower | protocol.GroupUpdateFlagPetAuras | protocol.GroupUpdateFlagVehicleSeat
	value := protocol.PartyMemberStatsFull{GUID: guid, UpdateFlags: flags, Status: 0x41, MaximumHealth: 123, PowerType: 2, MaximumPower: 456, Zone: 42, AuraMask: 1 << 2, PetGUID: 0xF140000000004321, PetName: "", PetAuraMask: 1 << 63}
	value.Auras[2] = protocol.PartyMemberAura{SpellID: 0, Flags: protocol.AuraFlagPositive}
	value.PetAuras[63] = protocol.PartyMemberAura{SpellID: 7654, Flags: protocol.AuraFlagCaster}
	packet := protocol.NewBuffer(256)
	packet.WriteU8(0)
	packet.WritePackedGUID(guid)
	packet.WriteU32(flags)
	packet.WriteU16(value.Status)
	packet.WriteU32(value.Health)
	packet.WriteU32(value.MaximumHealth)
	packet.WriteU8(value.PowerType)
	packet.WriteU16(value.CurrentPower)
	packet.WriteU16(value.MaximumPower)
	packet.WriteU16(value.Level)
	packet.WriteU16(value.Zone)
	packet.WriteU16(value.X)
	packet.WriteU16(value.Y)
	packet.WriteU64(value.AuraMask)
	packet.WriteU32(value.Auras[2].SpellID)
	packet.WriteU8(value.Auras[2].Flags)
	packet.WriteU64(value.PetGUID)
	packet.WriteCString(value.PetName)
	packet.WriteU16(value.PetModelID)
	packet.WriteU32(value.PetHealth)
	packet.WriteU32(value.PetMaximumHealth)
	packet.WriteU8(value.PetPowerType)
	packet.WriteU16(value.PetCurrentPower)
	packet.WriteU16(value.PetMaximumPower)
	packet.WriteU64(value.PetAuraMask)
	packet.WriteU32(value.PetAuras[63].SpellID)
	packet.WriteU8(value.PetAuras[63].Flags)
	packet.WriteU32(value.VehicleSeatID)
	if actual := protocol.BuildPartyMemberStatsFull(value); string(actual) != string(packet.Bytes()) {
		return fmt.Errorf("SMSG_PARTY_MEMBER_STATS_FULL field order or zero-value encoding differs from the fixture")
	}
	offline := protocol.PartyMemberStatsFull{GUID: guid, UpdateFlags: protocol.GroupUpdateFlagStatus}
	packet = protocol.NewBuffer(16)
	packet.WriteU8(0)
	packet.WritePackedGUID(guid)
	packet.WriteU32(protocol.GroupUpdateFlagStatus)
	packet.WriteU16(0)
	if actual := protocol.BuildPartyMemberStatsFull(offline); string(actual) != string(packet.Bytes()) {
		return fmt.Errorf("offline SMSG_PARTY_MEMBER_STATS_FULL differs from the source fixture")
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
		auraType                                uint32
		hasFlightSpeedAura, hasFlightCapability bool
		run, flight, canFly                     bool
	}{{31, false, false, true, false, false}, {33, false, false, true, true, false}, {78, false, false, true, false, false}, {78, true, false, true, true, false}, {78, true, true, true, true, true}, {201, false, false, false, false, true}, {206, false, false, false, true, false}, {207, false, false, false, true, true}, {211, false, false, false, true, false}} {
		run, flight, canFly := world.ResolveMovementSpeedUpdatePlan(test.auraType, test.hasFlightSpeedAura, test.hasFlightCapability)
		if run != test.run || flight != test.flight || canFly != test.canFly {
			return fmt.Errorf("aura type %d flight-speed=%t can-fly=%t plan=(%t,%t,%t) want=(%t,%t,%t)", test.auraType, test.hasFlightSpeedAura, test.hasFlightCapability, run, flight, canFly, test.run, test.flight, test.canFly)
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
	enteredAt := time.Unix(1_800_000_000, 0)
	priorInstanceTimes := map[uint32]int64{3: enteredAt.Unix() - 1, 4: enteredAt.Unix() + 120}
	updatedInstanceTimes := world.ResolveAccountInstanceEnterTime(priorInstanceTimes, 4, enteredAt)
	if len(updatedInstanceTimes) != 1 || updatedInstanceTimes[4] != enteredAt.Unix()+120 || priorInstanceTimes[3] != enteredAt.Unix()-1 {
		return fmt.Errorf("instance-entry timer pruning or non-refresh behavior mismatch: %#v", updatedInstanceTimes)
	}
	newInstanceTimes := world.ResolveAccountInstanceEnterTime(updatedInstanceTimes, 5, enteredAt)
	if newInstanceTimes[5] != enteredAt.Add(time.Hour).Unix() {
		return fmt.Errorf("new instance-entry timer=%d, want %d", newInstanceTimes[5], enteredAt.Add(time.Hour).Unix())
	}
	if err := validateAccountInstanceTimeDelta(priorInstanceTimes, newInstanceTimes, enteredAt.Unix()); err != nil {
		return fmt.Errorf("source account instance-time delta was rejected: %w", err)
	}
	if validateAccountInstanceTimeDelta(map[uint32]int64{4: enteredAt.Unix() + 120}, map[uint32]int64{}, enteredAt.Unix()) == nil {
		return fmt.Errorf("active account instance lock removal was accepted")
	}
	if validateAccountInstanceTimeDelta(map[uint32]int64{4: enteredAt.Unix() + 120}, map[uint32]int64{4: enteredAt.Add(time.Hour).Unix()}, enteredAt.Unix()) == nil {
		return fmt.Errorf("account instance lock refresh on re-entry was accepted")
	}
	if value := (wotlk.SpellEffect{BasePoints: 10}).CalcValue(); value != 10 {
		return fmt.Errorf("unscaled DBC spell effect value=%d, want 10", value)
	}
	levelEffect := wotlk.SpellEffect{BasePoints: 10, RealPointsPerLevel: 2}
	levelSpell := wotlk.Spell{BaseLevel: 1, SpellLevel: 2, MaxLevel: 5}
	if value := levelEffect.CalcValueForLevel(levelSpell, 4); value != 14 || levelEffect.CalcValueForLevel(levelSpell, 10) != 16 {
		return fmt.Errorf("DBC spell level value=%d cap=%d, want 14/16", value, levelEffect.CalcValueForLevel(levelSpell, 10))
	}
	for _, effect := range []wotlk.SpellEffect{{BasePoints: 10, DieSides: 3}, {BasePoints: 10, DieSides: -2}} {
		minValue, maxValue := effect.CalcValueRangeForLevel(wotlk.Spell{}, 0)
		for range 32 {
			if value := effect.CalcValue(); value < minValue || value > maxValue {
				return fmt.Errorf("DBC spell die-side value=%d outside [%d,%d]", value, minValue, maxValue)
			}
		}
	}
	if rate := world.ResolveGroupXPRate(3, false); rate != 1.166 {
		return fmt.Errorf("three-member XP group rate=%v, want 1.166", rate)
	}
	if rate := world.ResolveGroupXPRate(4, false); rate != 1.3 {
		return fmt.Errorf("four-member XP group rate=%v, want 1.3", rate)
	}
	if rate := world.ResolveGroupXPRate(5, true); rate != 1 {
		return fmt.Errorf("raid XP group rate=%v, want 1", rate)
	}
	if share := world.ResolveGroupXPShare(1000, 10, 30, 10, 10, 1.166); share != 388 {
		return fmt.Errorf("full-XP three-member share=%d, want 388", share)
	}
	if share := world.ResolveGroupXPShare(1000, 10, 40, 20, 10, 1.166); share != 146 {
		return fmt.Errorf("mixed-gray group share=%d, want 146", share)
	}
	if share := world.ResolveGroupXPShare(1000, 20, 40, 20, 10, 1.166); share != 0 {
		return fmt.Errorf("gray member group share=%d, want 0", share)
	}
	botRoster := []world.NpcBotRuntimeState{{GUID: 1, OwnerGUID: 7, Entry: 101}, {GUID: 2, OwnerGUID: 7, Entry: 102}, {GUID: 3, OwnerGUID: 7, Entry: 103, PetID: 44}, {GUID: 4, OwnerGUID: 8, Entry: 101}, {GUID: 5, OwnerGUID: 7, Entry: 999}, {GUID: 1, OwnerGUID: 7, Entry: 101}}
	botEntries := map[uint32]struct{}{101: {}, 102: {}, 103: {}}
	if count := world.ResolveNpcBotRuntimeCount(7, botEntries, botRoster); count != 2 || world.ResolveNpcBotXPGain(1000, count, 20) != 800 {
		return fmt.Errorf("runtime NPCBot XP reduction count=%d, XP=%d; want count=2 XP=800", count, world.ResolveNpcBotXPGain(1000, count, 20))
	}
	if world.ResolveNpcBotXPGain(1000, 1, 20) != 1000 || world.ResolveNpcBotXPGain(1000, 3, 20) != 600 || world.ResolveNpcBotXPGain(1000, 8, 90) != 100 {
		return fmt.Errorf("NPCBot XP reduction did not match per-bot percentage and 10%% floor")
	}
	if !world.ResolveGroupXPMapEligibility(571, 571, 1, 1) || world.ResolveGroupXPMapEligibility(571, 571, 1, 2) || world.ResolveGroupXPMapEligibility(571, 572, 1, 1) {
		return fmt.Errorf("group XP map/instance eligibility did not match same-instance reward rules")
	}
	if distance := world.ResolveGroupXPDistance(80, 1.5, 1.5); distance != 77 || world.ResolveGroupXPDistance(2, 1.5, 1.5) != 0 {
		return fmt.Errorf("group XP distance did not subtract object combat reach: %v", distance)
	}
	cooldownStart := time.Unix(100, 0)
	spellEnd, categoryEnd, hasCooldown := world.ResolvePetCooldownEnds(cooldownStart, 0, 5000)
	if !hasCooldown || !spellEnd.Equal(categoryEnd) || spellEnd.Sub(cooldownStart) != 5*time.Second {
		return fmt.Errorf("category-only cooldown ends spell=%v category=%v has=%t", spellEnd, categoryEnd, hasCooldown)
	}
	if _, _, hasCooldown := world.ResolvePetCooldownEnds(cooldownStart, 0, 0); hasCooldown {
		return fmt.Errorf("zero-duration pet spell produced a cooldown")
	}
	if gain := world.ResolvePetFocusRegen(1, []world.PetFocusModifier{{AuraType: 85, Amount: 25}, {AuraType: 110, Amount: 50}}); gain != 56 {
		return fmt.Errorf("source Focus aura formula=%d, want 56", gain)
	}
	if gain := world.ResolvePetFocusRegen(1, []world.PetFocusModifier{{AuraType: 85, Amount: 25, StackGroup: 100}, {AuraType: 85, Amount: 50, StackGroup: 100}}); gain != 64 {
		return fmt.Errorf("same-effect flat Focus aura stack=%d, want 64", gain)
	}
	if gain := world.ResolvePetFocusRegen(1, []world.PetFocusModifier{{AuraType: 110, Amount: 25, StackGroup: 100}, {AuraType: 110, Amount: 50, StackGroup: 100}}); gain != 36 {
		return fmt.Errorf("same-effect percent Focus aura stack=%d, want 36", gain)
	}
	if gain := world.ResolvePetFocusRegen(1, []world.PetFocusModifier{{AuraType: 110, Amount: 50, StackGroup: 100}, {AuraType: 110, Amount: 50, StackGroup: 101}}); gain != 54 {
		return fmt.Errorf("independent percent Focus aura stack=%d, want 54", gain)
	}
	if multiplier := world.ResolveAuraPercentMultiplierByType([]world.PetFocusModifier{{AuraType: 200, Amount: 50}, {AuraType: 200, Amount: 50}}, 200); multiplier != 2.25 {
		return fmt.Errorf("independent XP-aura multiplier=%v, want 2.25", multiplier)
	}
	if multiplier := world.ResolveAuraPercentMultiplierByType([]world.PetFocusModifier{{AuraType: 200, Amount: 25, StackGroup: 100}, {AuraType: 200, Amount: 50, StackGroup: 100}}, 200); multiplier != 1.5 {
		return fmt.Errorf("same-effect XP-aura multiplier=%v, want 1.5", multiplier)
	}
	if gain := world.ResolvePetFocusRegen(1, []world.PetFocusModifier{{AuraType: 85, Amount: -25}}); gain != 858997 {
		return fmt.Errorf("source signed flat Focus arithmetic=%d, want 858997", gain)
	}
	multiEffect := wotlk.Spell{Effects: [3]wotlk.SpellEffect{{Effect: 6, Aura: 20}, {Effect: 6, Aura: 118}}}
	if mask := world.PetAuraEffectMask(multiEffect); mask != 3 {
		return fmt.Errorf("mapped pet aura effect mask=%d, want 3", mask)
	}
	if amount := world.ResolveOwnerPetAuraAmount(35696, 0, 10, 4, [5]uint32{0, 0, 24, 33}); amount != 2 {
		return fmt.Errorf("Demonic Knowledge scaling=%d, want 2", amount)
	}
	modifiedState := world.PetRuntimeState{PowerType: 2, UnitFlags2: 0x00000800, MaxPowers: [7]uint32{0, 0, 100}, FocusRegenTimer: 4 * time.Second}
	modifiedState, modifiedFields := world.AdvancePetRuntimeWithAuras(modifiedState, 4*time.Second, 1, []world.PetFocusModifier{{AuraType: 85, Amount: 25}, {AuraType: 110, Amount: 50}})
	if modifiedState.Powers[2] != 56 || modifiedFields[27] != 56 {
		return fmt.Errorf("aura-modified Focus tick=%d fields=%v, want 56", modifiedState.Powers[2], modifiedFields)
	}
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
	defaults := config.Default()
	if defaults.MaxGroupXPDistance != 74 || defaults.XPRateKill != 1 || defaults.XPRateBattlegroundKill != 1 {
		return fmt.Errorf("default group/kill XP config=(%v,%v,%v)", defaults.MaxGroupXPDistance, defaults.XPRateKill, defaults.XPRateBattlegroundKill)
	}
	loaded, err := config.Load("configs/worldserver.conf.dist")
	if err != nil || loaded.FocusRate != 1 || loaded.MaxGroupXPDistance != 74 || loaded.XPRateKill != 1 || loaded.XPRateBattlegroundKill != 1 {
		return fmt.Errorf("worldserver group/focus XP config=(%v,%v,%v,%v) err=%v", loaded.FocusRate, loaded.MaxGroupXPDistance, loaded.XPRateKill, loaded.XPRateBattlegroundKill, err)
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
	previousDistance, hadPreviousDistance := os.LookupEnv("MORENOCORE_MAX_GROUP_XP_DISTANCE")
	previousKillRate, hadPreviousKillRate := os.LookupEnv("MORENOCORE_RATE_XP_KILL")
	previousBGRRate, hadPreviousBGRRate := os.LookupEnv("MORENOCORE_RATE_XP_BATTLEGROUND_KILL")
	if err := os.Setenv("MORENOCORE_MAX_GROUP_XP_DISTANCE", "91"); err != nil {
		return err
	}
	if err := os.Setenv("MORENOCORE_RATE_XP_KILL", "1.25"); err != nil {
		return err
	}
	if err := os.Setenv("MORENOCORE_RATE_XP_BATTLEGROUND_KILL", "2"); err != nil {
		return err
	}
	loaded.ApplyEnv()
	if hadPreviousDistance {
		_ = os.Setenv("MORENOCORE_MAX_GROUP_XP_DISTANCE", previousDistance)
	} else {
		_ = os.Unsetenv("MORENOCORE_MAX_GROUP_XP_DISTANCE")
	}
	if hadPreviousKillRate {
		_ = os.Setenv("MORENOCORE_RATE_XP_KILL", previousKillRate)
	} else {
		_ = os.Unsetenv("MORENOCORE_RATE_XP_KILL")
	}
	if hadPreviousBGRRate {
		_ = os.Setenv("MORENOCORE_RATE_XP_BATTLEGROUND_KILL", previousBGRRate)
	} else {
		_ = os.Unsetenv("MORENOCORE_RATE_XP_BATTLEGROUND_KILL")
	}
	if loaded.MaxGroupXPDistance != 91 || loaded.XPRateKill != 1.25 || loaded.XPRateBattlegroundKill != 2 {
		return fmt.Errorf("environment group/kill XP config=(%v,%v,%v)", loaded.MaxGroupXPDistance, loaded.XPRateKill, loaded.XPRateBattlegroundKill)
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

func checkPetFoodRules() error {
	if !world.PetFoodInDiet(1, 1) || !world.PetFoodInDiet(32, 1<<31) || world.PetFoodInDiet(0, ^uint32(0)) || world.PetFoodInDiet(33, ^uint32(0)) || world.PetFoodInDiet(2, 1) {
		return fmt.Errorf("pet food mask bit selection mismatch")
	}
	for _, test := range []struct{ petLevel, itemLevel, benefit uint32 }{{60, 55, 35000}, {61, 55, 17000}, {69, 55, 8000}, {70, 55, 0}} {
		if got := world.PetFoodBenefitLevel(test.petLevel, test.itemLevel); got != test.benefit {
			return fmt.Errorf("pet food benefit pet=%d item=%d got=%d want=%d", test.petLevel, test.itemLevel, got, test.benefit)
		}
	}
	data := wotlk.NewStore(filepath.Join(config.Default().GameDataDir, "dbc"))
	if mask, found, err := data.CreatureFamilyPetFoodMask(21); err != nil || !found || mask != 58 {
		return fmt.Errorf("CreatureFamily.dbc family 21 food mask=%d found=%t err=%v want=58", mask, found, err)
	}
	return nil
}

func checkExpectedCharacterStateDelta() error {
	before := map[string]characterTableSnapshot{
		"characters":     {Rows: 1, Columns: map[string]string{"level": "a", "position_x": "b", "health": "saved-health", "playerFlags": "saved-flags", "power1": "saved-over-max", "rest_bonus": "saved-rest", "taximask": "saved-mask", "zone": "stale-zone", "at_login": "first-login", "equipmentCache": "saved-cache", "knownTitles": "without-trailing-space", "extra_flags": "saved-gm-state", "map": "stored-transport-map", "trans_x": "old-x", "trans_y": "old-y", "trans_z": "old-z", "trans_o": "old-o", "transguid": "transport"}},
		"character_aura": {Rows: 2, Columns: map[string]string{"remainTime": "c"}},
	}
	after := map[string]characterTableSnapshot{
		"characters":     {Rows: 1, Columns: map[string]string{"level": "a", "position_x": "d", "health": "derived-max-health", "playerFlags": "restored-group-state", "power1": "derived-max-power", "rest_bonus": "calculated-rest", "taximask": "race-level-node-mask", "zone": "resolved-zone", "at_login": "cleared-first-login", "equipmentCache": "serialized-cache", "knownTitles": "source-serialized", "extra_flags": "config-derived-gm-state", "map": "live-transport-map", "trans_x": "new-x", "trans_y": "new-y", "trans_z": "new-z", "trans_o": "new-o", "transguid": "transport"}},
		"character_aura": {Rows: 1, Columns: map[string]string{"remainTime": "e"}},
	}
	if err := validateCharacterStateDelta(before, after, false); err != nil {
		return fmt.Errorf("source-expected login/logout changes rejected: %w", err)
	}
	statsBefore := map[string]characterTableSnapshot{"character_stats": {Rows: 0, Digest: "empty", Columns: map[string]string{"guid": "empty", "maxhealth": "empty"}}}
	statsAfter := map[string]characterTableSnapshot{"character_stats": {Rows: 1, Digest: "saved", Columns: map[string]string{"guid": "guid-8", "maxhealth": "health-8"}}}
	if validateCharacterStateDelta(statsBefore, statsAfter, false) == nil {
		return fmt.Errorf("disabled character_stats replay accepted a new stats row")
	}
	if err := validateCharacterStateDeltaWithStats(statsBefore, statsAfter, false); err != nil {
		return fmt.Errorf("source-enabled character_stats row was rejected: %w", err)
	}
	statsAfter["character_stats"] = characterTableSnapshot{Rows: 1, Digest: "tampered", Columns: map[string]string{"guid": "guid-8", "private": "changed"}}
	if validateCharacterStateDeltaWithStats(statsBefore, statsAfter, false) == nil {
		return fmt.Errorf("unowned character_stats column change was accepted")
	}
	accountOnlineBefore := map[string]characterTableSnapshot{
		"account_characters":  {Rows: 2, Digest: "characters-before", Columns: map[string]string{"guid": "stable-guid-set", "online": "one-online"}},
		"auth_account_online": {Rows: 1, Digest: "auth-before", Columns: map[string]string{"online": "1"}},
	}
	accountOnlineAfter := map[string]characterTableSnapshot{
		"account_characters":  {Rows: 2, Digest: "characters-after", Columns: map[string]string{"guid": "stable-guid-set", "online": "all-offline"}},
		"auth_account_online": {Rows: 1, Digest: "auth-after", Columns: map[string]string{"online": "0"}},
	}
	if err := validateCharacterStateDelta(accountOnlineBefore, accountOnlineAfter, false); err != nil {
		return fmt.Errorf("source account-wide online reset was rejected: %w", err)
	}
	accountOnlineAfter["account_characters"] = characterTableSnapshot{Rows: 2, Digest: "characters-mutated", Columns: map[string]string{"guid": "changed-guid-set", "online": "all-offline"}}
	if validateCharacterStateDelta(accountOnlineBefore, accountOnlineAfter, false) == nil {
		return fmt.Errorf("unrelated account character identity change was accepted")
	}
	unchangedColumns := map[string]string{"first": "same", "second": "same"}
	rowCompositionBefore := map[string]characterTableSnapshot{"characters": {Rows: 1, Digest: "before", Columns: unchangedColumns}}
	rowCompositionAfter := map[string]characterTableSnapshot{"characters": {Rows: 1, Digest: "after", Columns: unchangedColumns}}
	if err := validateCharacterStateDelta(rowCompositionBefore, rowCompositionAfter, false); err == nil {
		return fmt.Errorf("row composition change with unchanged column digests was accepted")
	}
	after["characters"] = characterTableSnapshot{Rows: 1, Columns: map[string]string{"level": "f", "position_x": "d"}}
	if err := validateCharacterStateDelta(before, after, false); err == nil {
		return fmt.Errorf("unclassified characters.level mutation was accepted")
	}
	feedBefore := map[string]characterTableSnapshot{
		"character_inventory":      {Rows: 1, Digest: "inventory-before", Columns: map[string]string{"guid": "owner", "bag": "0", "slot": "29", "item": "food"}, InventoryRows: map[string]inventoryRowSnapshot{"1:0:29:99": {ItemGUID: 99, OwnerGUID: 1, Slot: 29, Digest: "inventory-row"}}},
		"inventory_item_instances": {Rows: 1, Digest: "item-before", Columns: map[string]string{"guid": "food", "itemEntry": "entry", "owner_guid": "owner", "creatorGuid": "zero", "giftCreatorGuid": "zero", "count": "1", "duration": "0", "charges": "", "flags": "0", "enchantments": "", "randomPropertyId": "0", "durability": "100", "playedTime": "0", "text": ""}, InventoryRows: map[string]inventoryRowSnapshot{"99": {ItemGUID: 99, OwnerGUID: 1, ItemEntry: 17194, Count: 1, ItemTemplateValid: true, Digest: "item-row-before", DigestWithoutCount: "item-metadata"}}},
		"character_pet":            {Rows: 1, Digest: "pet-before", Columns: map[string]string{"level": "53", "exp": "0", "curhappiness": "0"}},
		"pet_aura":                 {Rows: 0, Digest: "aura-before", Columns: map[string]string{}},
	}
	feedAfter := map[string]characterTableSnapshot{
		"character_inventory":      {Rows: 0, Digest: "inventory-after", Columns: map[string]string{"guid": "", "bag": "", "slot": "", "item": ""}, InventoryRows: map[string]inventoryRowSnapshot{}},
		"inventory_item_instances": {Rows: 0, Digest: "item-after", Columns: map[string]string{"guid": "", "itemEntry": "", "owner_guid": "", "creatorGuid": "", "giftCreatorGuid": "", "count": "", "duration": "", "charges": "", "flags": "", "enchantments": "", "randomPropertyId": "", "durability": "", "playedTime": "", "text": ""}, InventoryRows: map[string]inventoryRowSnapshot{}},
		"character_pet":            {Rows: 1, Digest: "pet-after", Columns: map[string]string{"level": "53", "exp": "0", "curhappiness": "35000"}},
		"pet_aura":                 {Rows: 1, Digest: "aura-after", Columns: map[string]string{"guid": "pet", "casterGuid": "owner", "spell": "1539", "effectMask": "1", "recalculateMask": "0", "stackCount": "1", "amount0": "35000", "amount1": "0", "amount2": "0", "base_amount0": "34999", "base_amount1": "0", "base_amount2": "0", "maxDuration": "30000", "remainTime": "30000", "remainCharges": "0", "critChance": "0", "applyResilience": "0"}},
	}
	invalidInventoryBefore := map[string]characterTableSnapshot{"character_inventory": feedBefore["character_inventory"], "inventory_item_instances": feedBefore["inventory_item_instances"]}
	invalidInventoryAfter := map[string]characterTableSnapshot{"character_inventory": feedAfter["character_inventory"], "inventory_item_instances": feedAfter["inventory_item_instances"]}
	invalidItem := invalidInventoryBefore["inventory_item_instances"]
	invalidItem.InventoryRows = map[string]inventoryRowSnapshot{"99": {ItemGUID: 99, OwnerGUID: 1, ItemEntry: 0, Count: 1, Digest: "invalid-item", DigestWithoutCount: "invalid-metadata"}}
	invalidInventoryBefore["inventory_item_instances"] = invalidItem
	if err := validateCharacterStateDelta(invalidInventoryBefore, invalidInventoryAfter, false); err != nil {
		return fmt.Errorf("source invalid-item cleanup was rejected: %w", err)
	}
	missingTemplateBefore := map[string]characterTableSnapshot{"character_inventory": invalidInventoryBefore["character_inventory"], "inventory_item_instances": invalidInventoryBefore["inventory_item_instances"]}
	missingTemplateItem := missingTemplateBefore["inventory_item_instances"]
	missingTemplateRow := missingTemplateItem.InventoryRows["99"]
	missingTemplateRow.ItemEntry = 999999
	missingTemplateRow.ItemTemplateValid = false
	missingTemplateItem.InventoryRows = map[string]inventoryRowSnapshot{"99": missingTemplateRow}
	missingTemplateBefore["inventory_item_instances"] = missingTemplateItem
	if err := validateCharacterStateDelta(missingTemplateBefore, invalidInventoryAfter, false); err != nil {
		return fmt.Errorf("source missing-template item cleanup was rejected: %w", err)
	}
	if err := validateInventoryStateDelta(feedBefore, feedAfter, false); err == nil {
		return fmt.Errorf("valid non-buyback inventory deletion without pet-feed authorization was accepted")
	}
	if err := validateCharacterStateDelta(feedBefore, feedAfter, false); err == nil {
		return fmt.Errorf("pet happiness update outside pet-feed replay was accepted")
	}
	if err := validateCharacterStateDelta(feedBefore, feedAfter, true); err != nil {
		return fmt.Errorf("pet-feed inventory consumption was rejected: %w", err)
	}
	stackBefore := map[string]characterTableSnapshot{"character_inventory": {InventoryRows: map[string]inventoryRowSnapshot{"1:0:29:101": {ItemGUID: 101, OwnerGUID: 1, Slot: 29, Digest: "inventory-row"}}}, "inventory_item_instances": {InventoryRows: map[string]inventoryRowSnapshot{"101": {ItemGUID: 101, OwnerGUID: 1, ItemEntry: 17194, Count: 2, ItemTemplateValid: true, Digest: "stack-two", DigestWithoutCount: "stack-metadata"}}}}
	stackAfter := map[string]characterTableSnapshot{"character_inventory": {InventoryRows: map[string]inventoryRowSnapshot{"1:0:29:101": {ItemGUID: 101, OwnerGUID: 1, Slot: 29, Digest: "inventory-row"}}}, "inventory_item_instances": {InventoryRows: map[string]inventoryRowSnapshot{"101": {ItemGUID: 101, OwnerGUID: 1, ItemEntry: 17194, Count: 1, ItemTemplateValid: true, Digest: "stack-one", DigestWithoutCount: "stack-metadata"}}}}
	if validateInventoryStateDelta(stackBefore, stackAfter, false) == nil {
		return fmt.Errorf("inventory stack decrement without pet-feed authorization was accepted")
	}
	if err := validateInventoryStateDelta(stackBefore, stackAfter, true); err != nil {
		return fmt.Errorf("one-unit pet-feed stack decrement was rejected: %w", err)
	}
	zeroCountBefore := map[string]characterTableSnapshot{"character_inventory": feedBefore["character_inventory"], "inventory_item_instances": feedBefore["inventory_item_instances"]}
	zeroCountItem := zeroCountBefore["inventory_item_instances"]
	zeroCountRow := zeroCountItem.InventoryRows["99"]
	zeroCountRow.Count, zeroCountRow.Digest = 0, "zero-count-item"
	zeroCountItem.InventoryRows = map[string]inventoryRowSnapshot{"99": zeroCountRow}
	zeroCountBefore["inventory_item_instances"] = zeroCountItem
	if validateInventoryStateDelta(zeroCountBefore, feedAfter, false) == nil {
		return fmt.Errorf("valid zero-count item removal was accepted as source invalid-item cleanup")
	}
	buybackBefore := map[string]characterTableSnapshot{"character_inventory": {InventoryRows: map[string]inventoryRowSnapshot{"1:0:74:102": {ItemGUID: 102, OwnerGUID: 1, Slot: 74, Digest: "buyback-row"}}}, "inventory_item_instances": {InventoryRows: map[string]inventoryRowSnapshot{"102": {ItemGUID: 102, OwnerGUID: 1, ItemEntry: 17194, Count: 1, ItemTemplateValid: true, Digest: "buyback-instance", DigestWithoutCount: "buyback-metadata"}}}}
	buybackAfter := map[string]characterTableSnapshot{"character_inventory": {InventoryRows: map[string]inventoryRowSnapshot{}}, "inventory_item_instances": {InventoryRows: map[string]inventoryRowSnapshot{}}}
	if err := validateInventoryStateDelta(buybackBefore, buybackAfter, false); err != nil {
		return fmt.Errorf("source buyback cleanup was rejected: %w", err)
	}
	danglingInventory := inventoryRowSnapshot{ItemGUID: 103, OwnerGUID: 1, Bag: 900, Slot: 12, Digest: "dangling-link"}
	danglingBefore := map[string]characterTableSnapshot{"character_inventory": {InventoryRows: map[string]inventoryRowSnapshot{"1:900:12:103": danglingInventory}}, "inventory_item_instances": {InventoryRows: map[string]inventoryRowSnapshot{}}}
	danglingAfter := map[string]characterTableSnapshot{"character_inventory": {InventoryRows: map[string]inventoryRowSnapshot{"1:900:12:103": danglingInventory}}, "inventory_item_instances": {InventoryRows: map[string]inventoryRowSnapshot{}}}
	if err := validateInventoryStateDelta(danglingBefore, danglingAfter, false); err != nil {
		return fmt.Errorf("source-preserved dangling inventory reference was rejected: %w", err)
	}
	danglingRemoved := map[string]characterTableSnapshot{"character_inventory": {InventoryRows: map[string]inventoryRowSnapshot{}}, "inventory_item_instances": {InventoryRows: map[string]inventoryRowSnapshot{}}}
	if validateInventoryStateDelta(danglingBefore, danglingRemoved, false) == nil {
		return fmt.Errorf("dangling inventory reference cleanup without a source load row was accepted")
	}
	emptyGUIDLink := inventoryRowSnapshot{OwnerGUID: 1, Bag: 0, Slot: 30, Digest: "zero-item-link"}
	emptyGUIDBefore := map[string]characterTableSnapshot{"character_inventory": {InventoryRows: map[string]inventoryRowSnapshot{"1:0:30:0": emptyGUIDLink}}, "inventory_item_instances": {InventoryRows: map[string]inventoryRowSnapshot{}}}
	if err := validateInventoryStateDelta(emptyGUIDBefore, emptyGUIDBefore, false); err != nil {
		return fmt.Errorf("source-unloaded zero-item inventory reference was rejected: %w", err)
	}
	emptyGUIDAfter := map[string]characterTableSnapshot{"character_inventory": {InventoryRows: map[string]inventoryRowSnapshot{}}, "inventory_item_instances": {InventoryRows: map[string]inventoryRowSnapshot{}}}
	if validateInventoryStateDelta(emptyGUIDBefore, emptyGUIDAfter, false) == nil {
		return fmt.Errorf("zero-item inventory reference deletion was accepted outside its source load path")
	}
	cooldownBefore := map[string]characterTableSnapshot{"character_spell_cooldown": {Rows: 1, Columns: map[string]string{"guid": "owner", "spell": "expired", "item": "0", "time": "old", "categoryId": "0", "categoryEnd": "old"}}}
	cooldownExpired := map[string]characterTableSnapshot{"character_spell_cooldown": {Rows: 0, Columns: map[string]string{"guid": "", "spell": "", "item": "", "time": "", "categoryId": "", "categoryEnd": ""}}}
	if err := validateCharacterStateDelta(cooldownBefore, cooldownExpired, false); err != nil {
		return fmt.Errorf("source expired cooldown cleanup was rejected: %w", err)
	}
	cooldownUnknown := map[string]characterTableSnapshot{"character_spell_cooldown": {Rows: 1, Columns: map[string]string{"guid": "owner", "spell": "expired", "item": "0", "time": "old", "categoryId": "0", "categoryEnd": "old", "unexpected": "unclassified"}}}
	if validateCharacterStateDelta(cooldownBefore, cooldownUnknown, false) == nil {
		return fmt.Errorf("unclassified spell cooldown column was accepted")
	}
	petCooldownKey := petSpellCooldownKey{GUID: 14, Spell: 24453}
	petCooldownBefore := map[string]characterTableSnapshot{"pet_spell_cooldown": {Rows: 1, Columns: map[string]string{"guid": "pet", "spell": "expired", "time": "old", "categoryId": "38", "categoryEnd": "old"}, PetSpellCooldowns: map[petSpellCooldownKey]petSpellCooldownSnapshot{petCooldownKey: {Time: time.Now().Unix() - 1, CategoryID: 38, CategoryEnd: time.Now().Unix() - 1}}}}
	petCooldownAfter := map[string]characterTableSnapshot{"pet_spell_cooldown": {Rows: 0, Columns: map[string]string{"guid": "", "spell": "", "time": "", "categoryId": "", "categoryEnd": ""}, PetSpellCooldowns: map[petSpellCooldownKey]petSpellCooldownSnapshot{}}}
	if err := validateCharacterStateDelta(petCooldownBefore, petCooldownAfter, false); err != nil {
		return fmt.Errorf("expired pet cooldown removal was rejected: %w", err)
	}
	activePetCooldownBefore := map[string]characterTableSnapshot{"pet_spell_cooldown": {Rows: 1, Columns: petCooldownBefore["pet_spell_cooldown"].Columns, PetSpellCooldowns: map[petSpellCooldownKey]petSpellCooldownSnapshot{petCooldownKey: {Time: time.Now().Unix() + 60, CategoryID: 38, CategoryEnd: time.Now().Unix() + 60}}}}
	if validateCharacterStateDelta(activePetCooldownBefore, petCooldownAfter, false) == nil {
		return fmt.Errorf("active pet cooldown removal was accepted")
	}
	loginAchievementBefore := map[string]characterTableSnapshot{
		"character_achievement":          {Rows: 0, Columns: map[string]string{}},
		"character_achievement_progress": {Rows: 0, Columns: map[string]string{}},
	}
	loginAchievementAfter := map[string]characterTableSnapshot{
		"character_achievement":          {Rows: 1, Columns: map[string]string{"guid": "owner", "achievement": "achievement", "date": "login-time"}},
		"character_achievement_progress": {Rows: 1, Columns: map[string]string{"guid": "owner", "criteria": "criteria", "counter": "1", "date": "login-time"}},
	}
	if err := validateCharacterStateDelta(loginAchievementBefore, loginAchievementAfter, false); err != nil {
		return fmt.Errorf("source ON_LOGIN achievement mutations were rejected: %w", err)
	}
	loginAchievementAfter["character_achievement"] = characterTableSnapshot{Rows: 1, Columns: map[string]string{"guid": "owner", "achievement": "achievement", "date": "login-time", "private": "unclassified"}}
	if err := validateCharacterStateDelta(loginAchievementBefore, loginAchievementAfter, false); err == nil {
		return fmt.Errorf("unclassified character achievement column was accepted")
	}
	defaultSpellBefore := map[string]characterTableSnapshot{"character_spell": {Rows: 0, Columns: map[string]string{}}}
	defaultSpellAfter := map[string]characterTableSnapshot{"character_spell": {Rows: 1, Columns: map[string]string{"guid": "owner", "spell": "skill-default", "active": "1", "disabled": "0"}}}
	if err := validateCharacterStateDelta(defaultSpellBefore, defaultSpellAfter, false); err != nil {
		return fmt.Errorf("source learned-default-spell mutation was rejected: %w", err)
	}
	defaultSpellAfter["character_spell"] = characterTableSnapshot{Rows: 1, Columns: map[string]string{"guid": "owner", "spell": "skill-default", "active": "1", "disabled": "0", "unexpected": "unclassified"}}
	if err := validateCharacterStateDelta(defaultSpellBefore, defaultSpellAfter, false); err == nil {
		return fmt.Errorf("unclassified character spell column was accepted")
	}
	invalidSkillBefore := map[string]characterTableSnapshot{"character_skills": {Rows: 1, Columns: map[string]string{"guid": "owner", "skill": "invalid-race-class", "value": "400", "max": "400"}}}
	invalidSkillAfter := map[string]characterTableSnapshot{"character_skills": {Rows: 0, Columns: map[string]string{"guid": "", "skill": "", "value": "", "max": ""}}}
	if err := validateCharacterStateDelta(invalidSkillBefore, invalidSkillAfter, false); err != nil {
		return fmt.Errorf("source invalid-skill removal was rejected: %w", err)
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

func checkPlayerStatsConfig() error {
	config, err := config.Load(filepath.Join("configs", "worldserver.conf.dist"))
	if err != nil {
		return fmt.Errorf("load default player stats configuration: %w", err)
	}
	if config.PlayerSaveStatsMinLevel != 0 || !config.PlayerSaveStatsSaveOnlyOnLogout {
		return fmt.Errorf("default PlayerSave.Stats config min_level=%d save_only_on_logout=%t, want 0/true", config.PlayerSaveStatsMinLevel, config.PlayerSaveStatsSaveOnlyOnLogout)
	}
	return nil
}

func checkAuraGUIDRepresentations() error {
	for _, test := range []struct {
		value any
		want  uint64
		ok    bool
	}{{int64(73), 73, true}, {uint64(math.MaxUint64), math.MaxUint64, true}, {[]byte("18446744073709551615"), math.MaxUint64, true}, {"18446744073709551615", math.MaxUint64, true}, {float64(9007199254740991), 9007199254740991, true}, {float64(9007199254740992), 0, false}, {float64(12.5), 0, false}, {float32(16777215), 16777215, true}, {float32(16777216), 0, false}} {
		got, ok := world.ParseAuraGUID(test.value)
		if got != test.want || ok != test.ok {
			return fmt.Errorf("aura GUID decoding type=%T value=%v => %d/%t, want %d/%t", test.value, test.value, got, ok, test.want, test.ok)
		}
	}
	return nil
}

func checkSpellPowerEnchantmentDBC() error {
	data := wotlk.NewStore(filepath.Join(config.Default().GameDataDir, "dbc"))
	file, err := data.File("SpellItemEnchantment")
	if err != nil {
		return fmt.Errorf("load SpellItemEnchantment.dbc: %w", err)
	}
	for index := 0; index < file.Records(); index++ {
		record, err := file.Record(index)
		if err != nil {
			return err
		}
		id, err := record.Uint32(0)
		if err != nil {
			return err
		}
		entry, found, err := data.SpellItemEnchantment(id)
		if err != nil {
			return err
		}
		if !found || entry.ConditionID != 0 || entry.RequiredSkillID != 0 || entry.MinLevel > 80 {
			continue
		}
		var want uint32
		for effectIndex, effect := range entry.Effects {
			if effect != 5 || entry.EffectArg[effectIndex] != 45 || entry.EffectPointsMin[effectIndex] == 0 {
				continue
			}
			want += entry.EffectPointsMin[effectIndex]
		}
		if want == 0 {
			continue
		}
		expected := sourceSpellPowerEnchant(entry, 80, 0, false, 0, false)
		if expected != want {
			return fmt.Errorf("SpellItemEnchantment.dbc id=%d source spell-power amount=%d, field sum=%d", id, expected, want)
		}
		if amount := world.ResolveEquippedSpellPowerEnchant(entry, 80, 0, false); amount != expected {
			return fmt.Errorf("SpellItemEnchantment.dbc id=%d spell-power amount=%d, want %d", id, amount, expected)
		}
		if amount := world.ResolveEquippedSpellPowerEnchant(entry, 80, 0, true); amount != 0 {
			return fmt.Errorf("broken item retained spell-power enchant id=%d amount=%d", id, amount)
		}
		conditioned := entry
		conditioned.ConditionID = 1
		if amount := world.ResolveEquippedSpellPowerEnchant(conditioned, 80, 0, false); amount != 0 {
			return fmt.Errorf("conditioned item enchant id=%d applied without its condition", id)
		}
		skilled := entry
		skilled.RequiredSkillID, skilled.RequiredSkillRank = 164, 1
		if amount := world.ResolveEquippedSpellPowerEnchant(skilled, 80, 0, false); amount != 0 || world.ResolveEquippedSpellPowerEnchant(skilled, 80, 1, false) != want {
			return fmt.Errorf("required-skill gate for item enchant id=%d differs", id)
		}
		underLevel := entry
		underLevel.MinLevel = 81
		if amount := world.ResolveEquippedSpellPowerEnchant(underLevel, 80, 0, false); amount != 0 {
			return fmt.Errorf("under-level item enchant id=%d was applied", id)
		}
		return nil
	}
	return fmt.Errorf("SpellItemEnchantment.dbc contains no unconditioned, unskilled spell-power stat effect")
}

func checkRandomSuffixSpellPowerDBC() error {
	data := wotlk.NewStore(filepath.Join(config.Default().GameDataDir, "dbc"))
	factor := uint32(0)
	var factorLevel uint32
	for level := uint32(1); level <= 80 && factor == 0; level++ {
		factor = sourceItemSuffixFactor(data, level, 4, 13, 1)
		factorLevel = level
	}
	if factor == 0 {
		return fmt.Errorf("RandPropPoints.dbc has no epic weapon suffix factor")
	}
	if world.ResolveItemSuffixFactor(data, factorLevel, 4, 13, 1) != factor {
		return fmt.Errorf("Go item suffix factor differs from the source level/inventory/quality lookup")
	}
	file, err := data.File("ItemRandomSuffix")
	if err != nil {
		return fmt.Errorf("load ItemRandomSuffix.dbc: %w", err)
	}
	for index := 0; index < file.Records(); index++ {
		record, err := file.Record(index)
		if err != nil {
			return err
		}
		id, err := record.Uint32(0)
		if err != nil {
			return err
		}
		suffix, found, err := data.ItemRandomSuffix(id)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		for effectIndex, enchantID := range suffix.Enchantment {
			if enchantID == 0 || suffix.AllocationPct[effectIndex] == 0 {
				continue
			}
			entry, found, err := data.SpellItemEnchantment(enchantID)
			if err != nil {
				return err
			}
			if !found || entry.ConditionID != 0 || entry.MinLevel > 80 {
				continue
			}
			amount := suffix.AllocationPct[effectIndex] * factor / 10000
			expected := sourceSpellPowerEnchant(entry, 80, entry.RequiredSkillRank, false, amount, true)
			if expected == 0 {
				continue
			}
			if actual := world.ResolveRandomSuffixSpellPowerEnchant(entry, 80, entry.RequiredSkillRank, false, amount); actual != expected {
				return fmt.Errorf("random suffix DBC spell-power enchant %d amount=%d, source amount=%d", enchantID, actual, expected)
			}
			return nil
		}
	}
	return fmt.Errorf("ItemRandomSuffix.dbc contains no spell-power stat enchant for a qualified suffix fixture")
}

func sourceSpellPowerEnchant(enchantment wotlk.SpellItemEnchantmentEntry, level, skillValue uint32, broken bool, suffixAmount uint32, suffix bool) uint32 {
	if broken || enchantment.ConditionID != 0 || level < enchantment.MinLevel || skillValue < enchantment.RequiredSkillRank {
		return 0
	}
	var total uint32
	for index, effect := range enchantment.Effects {
		if effect != 5 || enchantment.EffectArg[index] != 45 {
			continue
		}
		amount := enchantment.EffectPointsMin[index]
		if amount == 0 && suffix {
			amount = suffixAmount
		}
		total += amount
	}
	return total
}

func sourceItemSuffixFactor(data *wotlk.Store, itemLevel, quality, inventoryType, randomSuffix uint32) uint32 {
	if randomSuffix == 0 || data == nil {
		return 0
	}
	points, found, err := data.RandPropPoints(itemLevel)
	if err != nil || !found {
		return 0
	}
	index := -1
	switch inventoryType {
	case 1, 4, 5, 7, 17, 20:
		index = 0
	case 3, 6, 8, 10, 12:
		index = 1
	case 2, 9, 11, 14, 16, 23:
		index = 2
	case 13, 21, 22:
		index = 3
	case 15, 25, 26:
		index = 4
	}
	if index < 0 {
		return 0
	}
	switch quality {
	case 2:
		return points.Good[index]
	case 3:
		return points.Superior[index]
	case 4:
		return points.Epic[index]
	default:
		return 0
	}
}

func sourceGemSocketEnchantActive(socketColor uint32, prismatic wotlk.SpellItemEnchantmentEntry, found bool, skillValue uint32) bool {
	return socketColor != 0 || found && (prismatic.RequiredSkillID == 0 || skillValue >= prismatic.RequiredSkillRank)
}

func validateCharacterStatsSpellPower(ctx context.Context, charactersDB, worldDB *sql.DB, data *wotlk.Store, guid uint64, trace protocoltrace.Trace) error {
	var level uint32
	if err := charactersDB.QueryRowContext(ctx, "SELECT level FROM characters WHERE guid = ?", guid).Scan(&level); err != nil {
		return fmt.Errorf("read character level for spell-power parity: %w", err)
	}
	skillValues := make(map[uint32]uint32)
	skillRows, err := charactersDB.QueryContext(ctx, "SELECT skill, value FROM character_skills WHERE guid = ?", guid)
	if err != nil {
		return fmt.Errorf("read character skills for spell-power parity: %w", err)
	}
	for skillRows.Next() {
		var skill, value uint32
		if err := skillRows.Scan(&skill, &value); err != nil {
			skillRows.Close()
			return err
		}
		skillValues[skill] = value
	}
	if err := skillRows.Err(); err != nil {
		skillRows.Close()
		return err
	}
	skillRows.Close()
	rows, err := charactersDB.QueryContext(ctx, `SELECT ii.itemEntry, COALESCE(ii.enchantments, ''), COALESCE(ii.durability, 0), COALESCE(ii.randomPropertyId, 0)
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot < 19`, guid)
	if err != nil {
		return fmt.Errorf("read equipped items for spell-power parity: %w", err)
	}
	template, err := worldDB.PrepareContext(ctx, `SELECT stat_type1, stat_value1, stat_type2, stat_value2,
		stat_type3, stat_value3, stat_type4, stat_value4, stat_type5, stat_value5,
		stat_type6, stat_value6, stat_type7, stat_value7, stat_type8, stat_value8,
		stat_type9, stat_value9, stat_type10, stat_value10, MaxDurability, ItemLevel, Quality, InventoryType, RandomSuffix,
		SocketColor_1, SocketColor_2, SocketColor_3 FROM item_template WHERE entry = ?`)
	if err != nil {
		rows.Close()
		return fmt.Errorf("prepare world item stats for spell-power parity: %w", err)
	}
	defer template.Close()
	var expected uint32
	for rows.Next() {
		var itemEntry uint32
		var enchantments string
		var durability uint32
		var randomPropertyID int32
		if err := rows.Scan(&itemEntry, &enchantments, &durability, &randomPropertyID); err != nil {
			rows.Close()
			return err
		}
		var statTypes, statValues [10]int64
		var maxDurability, itemLevel, quality, inventoryType, randomSuffix uint32
		var socketColors [3]uint32
		err := template.QueryRowContext(ctx, itemEntry).Scan(&statTypes[0], &statValues[0], &statTypes[1], &statValues[1], &statTypes[2], &statValues[2], &statTypes[3], &statValues[3], &statTypes[4], &statValues[4], &statTypes[5], &statValues[5], &statTypes[6], &statValues[6], &statTypes[7], &statValues[7], &statTypes[8], &statValues[8], &statTypes[9], &statValues[9], &maxDurability, &itemLevel, &quality, &inventoryType, &randomSuffix, &socketColors[0], &socketColors[1], &socketColors[2])
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			rows.Close()
			return err
		}
		for index := range statTypes {
			if statTypes[index] == 45 {
				expected += uint32(statValues[index])
			}
		}
		broken := maxDurability > 0 && durability == 0
		applyEnchant := func(enchantID, suffixAmount uint32, suffix bool) error {
			if enchantID == 0 {
				return nil
			}
			entry, found, err := data.SpellItemEnchantment(enchantID)
			if err != nil || !found {
				return err
			}
			skillValue := skillValues[entry.RequiredSkillID]
			amount := sourceSpellPowerEnchant(entry, level, skillValue, broken, 0, false)
			if suffix {
				amount = sourceSpellPowerEnchant(entry, level, skillValue, broken, suffixAmount, true)
			}
			expected += amount
			return nil
		}
		fields := strings.Fields(enchantments)
		for _, fieldIndex := range []int{0, 3} {
			if fieldIndex >= len(fields) {
				continue
			}
			enchantID, err := strconv.ParseUint(fields[fieldIndex], 10, 32)
			if err != nil || enchantID == 0 {
				continue
			}
			if err := applyEnchant(uint32(enchantID), 0, false); err != nil {
				rows.Close()
				return err
			}
		}
		var prismatic wotlk.SpellItemEnchantmentEntry
		var prismaticFound bool
		if len(fields) > 18 {
			if enchantID, err := strconv.ParseUint(fields[18], 10, 32); err == nil && enchantID != 0 {
				prismatic, prismaticFound, err = data.SpellItemEnchantment(uint32(enchantID))
				if err != nil {
					rows.Close()
					return err
				}
			}
		}
		for socket := range socketColors {
			fieldIndex := (socket + 2) * 3
			if fieldIndex >= len(fields) {
				continue
			}
			enchantID, err := strconv.ParseUint(fields[fieldIndex], 10, 32)
			if err != nil || enchantID == 0 || !sourceGemSocketEnchantActive(socketColors[socket], prismatic, prismaticFound, skillValues[prismatic.RequiredSkillID]) {
				continue
			}
			if err := applyEnchant(uint32(enchantID), 0, false); err != nil {
				rows.Close()
				return err
			}
		}
		if randomPropertyID > 0 {
			property, found, err := data.ItemRandomProperties(uint32(randomPropertyID))
			if err != nil {
				rows.Close()
				return err
			}
			if found {
				for _, enchantID := range property.Enchantment {
					if err := applyEnchant(enchantID, 0, false); err != nil {
						rows.Close()
						return err
					}
				}
			}
		} else if randomPropertyID < 0 {
			suffix, found, err := data.ItemRandomSuffix(uint32(-int64(randomPropertyID)))
			if err != nil {
				rows.Close()
				return err
			}
			if found {
				factor := sourceItemSuffixFactor(data, itemLevel, quality, inventoryType, randomSuffix)
				for index, enchantID := range suffix.Enchantment {
					amount := suffix.AllocationPct[index] * factor / 10000
					if err := applyEnchant(enchantID, amount, true); err != nil {
						rows.Close()
						return err
					}
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	var maxHealth uint32
	var maxPowers [7]uint32
	var stats [5]uint32
	var resistances [7]uint32
	var blockPct, dodgePct, parryPct, critPct, rangedCritPct, spellCritPct float32
	var attackPower, rangedAttackPower, savedSpellPower, resilience uint32
	if err := charactersDB.QueryRowContext(ctx, `SELECT maxhealth, maxpower1, maxpower2, maxpower3, maxpower4, maxpower5, maxpower6, maxpower7,
		strength, agility, stamina, intellect, spirit, armor, resHoly, resFire, resNature, resFrost, resShadow, resArcane,
		blockPct, dodgePct, parryPct, critPct, rangedCritPct, spellCritPct, attackPower, rangedAttackPower, spellPower, resilience
		FROM character_stats WHERE guid = ?`, guid).Scan(&maxHealth, &maxPowers[0], &maxPowers[1], &maxPowers[2], &maxPowers[3], &maxPowers[4], &maxPowers[5], &maxPowers[6], &stats[0], &stats[1], &stats[2], &stats[3], &stats[4], &resistances[0], &resistances[1], &resistances[2], &resistances[3], &resistances[4], &resistances[5], &resistances[6], &blockPct, &dodgePct, &parryPct, &critPct, &rangedCritPct, &spellCritPct, &attackPower, &rangedAttackPower, &savedSpellPower, &resilience); err != nil {
		return fmt.Errorf("read saved character stats: %w", err)
	}
	if savedSpellPower != expected {
		return fmt.Errorf("character_stats spellPower=%d, equipped calculation=%d", savedSpellPower, expected)
	}
	playerFields, err := selfPlayerCreateFields(trace, guid)
	if err != nil {
		return fmt.Errorf("read player create fields for character stats comparison: %w", err)
	}
	var race, class, gender, savedLevel int64
	if err := charactersDB.QueryRowContext(ctx, `SELECT race, class, gender, level FROM characters WHERE guid = ?`, guid).Scan(&race, &class, &gender, &savedLevel); err != nil {
		return fmt.Errorf("read saved character identity: %w", err)
	}
	identity := playerFields[23]
	if int64(uint8(identity)) != race || int64(uint8(identity>>8)) != class || int64(uint8(identity>>16)) != gender {
		return fmt.Errorf("player create identity bytes race/class/gender=%d/%d/%d, saved character=%d/%d/%d", uint8(identity), uint8(identity>>8), uint8(identity>>16), race, class, gender)
	}
	if int64(playerFields[54]) != savedLevel {
		return fmt.Errorf("player create level=%d, saved character level=%d", playerFields[54], savedLevel)
	}
	compareField := func(field int, column string, value uint32) error {
		if playerFields[field] != value {
			return fmt.Errorf("character_stats.%s=%d differs from player create field %d=%d", column, value, field, playerFields[field])
		}
		return nil
	}
	if err := compareField(32, "maxhealth", maxHealth); err != nil {
		return err
	}
	for index, value := range maxPowers {
		if err := compareField(33+index, fmt.Sprintf("maxpower%d", index+1), value); err != nil {
			return err
		}
	}
	for index, value := range stats {
		if err := compareField(84+index, []string{"strength", "agility", "stamina", "intellect", "spirit"}[index], value); err != nil {
			return err
		}
	}
	for index, value := range resistances {
		if err := compareField(99+index, []string{"armor", "resHoly", "resFire", "resNature", "resFrost", "resShadow", "resArcane"}[index], value); err != nil {
			return err
		}
	}
	for _, value := range []struct {
		field  int
		column string
		value  float32
	}{{1024, "blockPct", blockPct}, {1025, "dodgePct", dodgePct}, {1026, "parryPct", parryPct}, {1029, "critPct", critPct}, {1030, "rangedCritPct", rangedCritPct}, {1032, "spellCritPct", spellCritPct}} {
		if err := compareField(value.field, value.column, math.Float32bits(value.value)); err != nil {
			return err
		}
	}
	for _, value := range []struct {
		field  int
		column string
		value  uint32
	}{{123, "attackPower", attackPower}, {126, "rangedAttackPower", rangedAttackPower}, {1247, "resilience", resilience}} {
		if err := compareField(value.field, value.column, value.value); err != nil {
			return err
		}
	}
	return nil
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

func rejectWorldActivityBeforePlayerCreate(trace protocoltrace.Trace, start int) error {
	if start < 0 || start >= len(trace.Events) {
		return fmt.Errorf("login world-activity boundary has no login event")
	}
	playerGUID, err := loginPlayerGUID(trace.Events[start])
	if err != nil {
		return err
	}
	createIndex := -1
	for index := start + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) && event.Opcode != uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
			continue
		}
		created, err := containsPlayerCreate(event, playerGUID)
		if err != nil {
			return err
		}
		if created {
			createIndex = index
			break
		}
	}
	if createIndex < 0 {
		return nil
	}
	for index := start + 1; index < createIndex; index++ {
		event := trace.Events[index]
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		switch event.Opcode {
		case uint32(protocol.OpcodeSMSG_ATTACK_START), uint32(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), uint32(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), uint32(protocol.OpcodeSMSG_SPELL_GO):
			return fmt.Errorf("%s arrived before the self-player create update", opcodeName(event.Opcode))
		}
	}
	return nil
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
	if err := rejectWorldActivityBeforePlayerCreate(trace, start); err != nil {
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

func sourceLoginCinematicID(data *wotlk.Store, race, class uint32) uint32 {
	if data == nil {
		return 0
	}
	if entry, found, err := data.Class(class); err == nil && found && entry.CinematicSequence != 0 {
		return entry.CinematicSequence
	}
	if entry, found, err := data.Race(race); err == nil && found {
		return entry.CinematicSequence
	}
	return 0
}

func checkLoginCinematic(trace protocoltrace.Trace, expected, before, after uint32) error {
	count := 0
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_TRIGGER_CINEMATIC) {
			continue
		}
		count++
		payload, err := eventPayload(event)
		if err != nil {
			return err
		}
		reader := protocol.NewReader(payload)
		cinematicID, err := reader.ReadU32()
		if err != nil || reader.Remaining() != 0 || cinematicID != expected {
			return fmt.Errorf("login cinematic payload=%d, want DBC sequence %d", cinematicID, expected)
		}
	}
	wantCount := 0
	if before == 0 && expected != 0 {
		wantCount = 1
	}
	if count != wantCount {
		return fmt.Errorf("login cinematic triggers=%d, want %d for saved cinematic state %d", count, wantCount, before)
	}
	wantSaved := before
	if before == 0 {
		wantSaved = 1
	}
	if after != wantSaved {
		return fmt.Errorf("saved cinematic state after login=%d, want %d", after, wantSaved)
	}
	return nil
}

func checkLoginPlayerStartMessage(trace protocoltrace.Trace, playerGUID uint64, message string, expect bool) error {
	messageIndex, cinematicIndex, createIndex, count := -1, -1, -1, 0
	for index, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_TRIGGER_CINEMATIC) && cinematicIndex < 0 {
			cinematicIndex = index
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_MESSAGECHAT) {
			payload, err := eventPayload(event)
			if err != nil {
				return err
			}
			if strings.Contains(string(payload), message) {
				count++
				messageIndex = index
			}
		}
		if createIndex < 0 && (event.Opcode == uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) || event.Opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT)) {
			contains, err := containsPlayerCreate(event, playerGUID)
			if err != nil {
				return err
			}
			if contains {
				createIndex = index
			}
		}
	}
	wantCount := 0
	if expect {
		wantCount = 1
	}
	if count != wantCount {
		return fmt.Errorf("PlayerStart.String messages=%d, want %d", count, wantCount)
	}
	if expect && (cinematicIndex < 0 || messageIndex <= cinematicIndex || createIndex <= messageIndex) {
		return fmt.Errorf("PlayerStart.String must follow the cinematic and precede the player create")
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
		if event.Direction == protocoltrace.ServerToClient && event.Opcode == uint32(protocol.OpcodeSMSG_TRANSFER_PENDING) {
			break
		}
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
		if event.Direction == protocoltrace.ServerToClient && event.Opcode == uint32(protocol.OpcodeSMSG_TRANSFER_PENDING) {
			break
		}
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
			if guid != playerGUID {
				continue
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
			payload, err := eventPayload(event)
			if err != nil {
				return fmt.Errorf("SMSG_SPELL_GO payload: %w", err)
			}
			reader := protocol.NewReader(payload)
			casterGUID, err := reader.ReadPackedGUID()
			if err != nil {
				return fmt.Errorf("SMSG_SPELL_GO caster GUID: %w", err)
			}
			if casterGUID != playerGUID {
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
	var playerGUID uint64
	playerGUIDKnown := false
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
		if event.Opcode == uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL) {
			if !playerGUIDKnown {
				var err error
				playerGUID, err = loginPlayerGUID(trace.Events[start])
				if err != nil {
					return err
				}
				playerGUIDKnown = true
			}
			payload, err := eventPayload(event)
			if err != nil {
				return fmt.Errorf("SMSG_AURA_UPDATE_ALL: %w", err)
			}
			reader := protocol.NewReader(payload)
			targetGUID, err := reader.ReadPackedGUID()
			if err != nil {
				return fmt.Errorf("SMSG_AURA_UPDATE_ALL target GUID: %w", err)
			}
			if targetGUID != playerGUID {
				continue
			}
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

func checkCritterPetLoginTrace(trace protocoltrace.Trace) error {
	petCreate, petSpellBar, petTalents := false, false, false
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		switch event.Opcode {
		case uint32(protocol.OpcodeSMSG_PET_SPELLS):
			petSpellBar = true
		case uint32(protocol.OpcodeSMSG_TALENTS_INFO):
			payload, err := eventPayload(event)
			if err != nil {
				return err
			}
			petTalents = petTalents || len(payload) != 0 && payload[0] == 1
		case uint32(protocol.OpcodeSMSG_UPDATE_OBJECT), uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT):
			found, err := containsCritterPetCreate(event)
			if err != nil {
				return err
			}
			petCreate = petCreate || found
		}
	}
	if !petCreate || petSpellBar || petTalents {
		return fmt.Errorf("critter login create=%t pet-spell-packet=%t pet-talents-packet=%t", petCreate, petSpellBar, petTalents)
	}
	return nil
}

func checkAttachedTransportLogin(trace protocoltrace.Trace, playerGUID, savedTransportGUID uint64, savedOffset [4]float32) error {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) && event.Opcode != uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
			continue
		}
		movement, transportBeforePlayer, found, err := attachedTransportPlayerCreate(event, playerGUID)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if !movement.OnTransport || movement.TransportGUID&0xFFFFFFFF != savedTransportGUID&0xFFFFFFFF {
			return fmt.Errorf("player create transport flags=%#x GUID=%#x, saved transport=%#x", movement.Flags, movement.TransportGUID, savedTransportGUID)
		}
		if movement.TransportOffset != savedOffset {
			return fmt.Errorf("player create transport offsets=%v, saved offsets=%v", movement.TransportOffset, savedOffset)
		}
		if !transportBeforePlayer {
			return fmt.Errorf("attached transport create did not precede the passenger player create in one update")
		}
		for _, verify := range trace.Events {
			if verify.Direction != protocoltrace.ServerToClient || verify.Opcode != uint32(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD) {
				continue
			}
			payload, err := eventPayload(verify)
			if err != nil {
				return err
			}
			reader := protocol.NewReader(payload)
			if _, err := reader.ReadI32(); err != nil {
				return err
			}
			for index := range movement.Position {
				position, err := reader.ReadF32()
				if err != nil {
					return err
				}
				if position != movement.Position[index] {
					return fmt.Errorf("transported player verify/create position differs at component %d: %f != %f", index, position, movement.Position[index])
				}
			}
			if reader.Remaining() != 0 {
				return fmt.Errorf("transported player login verify has %d trailing bytes", reader.Remaining())
			}
			return nil
		}
		return fmt.Errorf("transported player login has no SMSG_LOGIN_VERIFY_WORLD")
	}
	return fmt.Errorf("transported player GUID %d create block was not found", playerGUID)
}

func attachedTransportPlayerCreate(event protocoltrace.Event, playerGUID uint64) (playerCreateMovement, bool, bool, error) {
	var movement playerCreateMovement
	payload, err := eventPayload(event)
	if err != nil {
		return movement, false, false, err
	}
	if event.Opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		payload, err = protocol.DecompressUpdatePayload(payload)
		if err != nil {
			return movement, false, false, err
		}
	}
	reader := protocol.NewReader(payload)
	blocks, err := reader.ReadU32()
	if err != nil {
		return movement, false, false, err
	}
	createdObjects := make([]struct {
		guid   uint64
		typeID uint8
	}, 0, blocks)
	for block := uint32(0); block < blocks; block++ {
		kind, err := reader.ReadU8()
		if err != nil {
			return movement, false, false, err
		}
		switch kind {
		case protocol.UpdateOutOfRangeObjects:
			count, err := reader.ReadU32()
			if err != nil {
				return movement, false, false, err
			}
			for index := uint32(0); index < count; index++ {
				if _, err := reader.ReadPackedGUID(); err != nil {
					return movement, false, false, err
				}
			}
		case protocol.UpdateCreateObject, protocol.UpdateCreateObject2:
			guid, err := reader.ReadPackedGUID()
			if err != nil {
				return movement, false, false, err
			}
			typeID, err := reader.ReadU8()
			if err != nil {
				return movement, false, false, err
			}
			flags, err := reader.ReadU16()
			if err != nil {
				return movement, false, false, err
			}
			if guid == playerGUID && typeID == 4 {
				movement, err = readPlayerCreateMovement(reader, flags)
				if err != nil {
					return movement, false, false, err
				}
				if _, _, err := readUpdateValues(reader); err != nil {
					return movement, false, false, err
				}
				for _, object := range createdObjects {
					if object.typeID == 5 && object.guid == movement.TransportGUID {
						return movement, true, true, nil
					}
				}
				return movement, false, true, nil
			}
			if err := skipCreateMovement(reader, flags); err != nil {
				return movement, false, false, err
			}
			if _, _, err := readUpdateValues(reader); err != nil {
				return movement, false, false, err
			}
			createdObjects = append(createdObjects, struct {
				guid   uint64
				typeID uint8
			}{guid, typeID})
		case protocol.UpdateValues:
			if err := skipValuesUpdate(reader); err != nil {
				return movement, false, false, err
			}
		case protocol.UpdateMovement:
			if err := skipMovementUpdate(reader); err != nil {
				return movement, false, false, err
			}
		default:
			return movement, false, false, fmt.Errorf("unsupported update block kind=%d", kind)
		}
	}
	return movement, false, false, nil
}

func containsCritterPetCreate(event protocoltrace.Event) (bool, error) {
	payload, err := eventPayload(event)
	if err != nil {
		return false, err
	}
	if event.Opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		payload, err = protocol.DecompressUpdatePayload(payload)
		if err != nil {
			return false, err
		}
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil {
		return false, err
	}
	for index := uint32(0); index < count; index++ {
		kind, err := reader.ReadU8()
		if err != nil {
			return false, err
		}
		switch kind {
		case protocol.UpdateOutOfRangeObjects:
			outOfRange, err := reader.ReadU32()
			if err != nil {
				return false, err
			}
			for guidIndex := uint32(0); guidIndex < outOfRange; guidIndex++ {
				if _, err := reader.ReadPackedGUID(); err != nil {
					return false, err
				}
			}
		case protocol.UpdateCreateObject, protocol.UpdateCreateObject2:
			guid, typeID, _, err := parseCreateObjectBlock(reader, 0)
			if err != nil {
				return false, err
			}
			if typeID == 3 && guid>>48 == 0xF140 {
				return true, nil
			}
		case protocol.UpdateValues:
			if err := skipValuesUpdate(reader); err != nil {
				return false, err
			}
		case protocol.UpdateMovement:
			if err := skipMovementUpdate(reader); err != nil {
				return false, err
			}
		default:
			return false, fmt.Errorf("unsupported update block kind=%d", kind)
		}
	}
	return false, nil
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
	for field := 0; field < 1326; field++ {
		if !updateMaskHas(mask, field) {
			continue
		}
		if values[field] == 0 {
			return guid, typeID, nil, fmt.Errorf("player create mask sets zero-valued field %d without a login field-notify source", field)
		}
		allowed := sourcePlayerCreateVisibility[field/32]&(uint32(1)<<uint(field%32)) != 0
		if !allowed && field >= 158 && field < 158+25*5 && (field-158)%5 == 0 {
			allowed = true
		}
		if !allowed {
			return guid, typeID, nil, fmt.Errorf("player update mask sets source-invisible self field %d", field)
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

func selfPlayerCreateFields(trace protocoltrace.Trace, playerGUID uint64) (map[int]uint32, error) {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) && event.Opcode != uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
			continue
		}
		payload, err := eventPayload(event)
		if err != nil {
			return nil, err
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
			payload, err = protocol.DecompressUpdatePayload(payload)
			if err != nil {
				return nil, err
			}
		}
		reader := protocol.NewReader(payload)
		count, err := reader.ReadU32()
		if err != nil {
			return nil, fmt.Errorf("update-object block count: %w", err)
		}
		for block := uint32(0); block < count; block++ {
			kind, err := reader.ReadU8()
			if err != nil {
				return nil, err
			}
			switch kind {
			case protocol.UpdateOutOfRangeObjects:
				outOfRange, err := reader.ReadU32()
				if err != nil {
					return nil, err
				}
				for index := uint32(0); index < outOfRange; index++ {
					if _, err := reader.ReadPackedGUID(); err != nil {
						return nil, err
					}
				}
			case protocol.UpdateCreateObject, protocol.UpdateCreateObject2:
				guid, typeID, fields, err := parseCreateObjectBlock(reader, playerGUID)
				if err != nil {
					return nil, err
				}
				if guid == playerGUID && typeID == 4 {
					return fields, nil
				}
			case protocol.UpdateValues:
				if err := skipValuesUpdate(reader); err != nil {
					return nil, err
				}
			case protocol.UpdateMovement:
				if err := skipMovementUpdate(reader); err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("unsupported update block kind=%d while extracting player stat fields", kind)
			}
		}
	}
	return nil, fmt.Errorf("self player create fields not found")
}

func countPlayerCreateBlocks(trace protocoltrace.Trace, playerGUID uint64) (int, error) {
	count := 0
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) && event.Opcode != uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
			continue
		}
		payload, err := eventPayload(event)
		if err != nil {
			return 0, err
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
			payload, err = protocol.DecompressUpdatePayload(payload)
			if err != nil {
				return 0, err
			}
		}
		reader := protocol.NewReader(payload)
		blocks, err := reader.ReadU32()
		if err != nil {
			return 0, fmt.Errorf("update-object block count: %w", err)
		}
		for block := uint32(0); block < blocks; block++ {
			kind, err := reader.ReadU8()
			if err != nil {
				return 0, fmt.Errorf("update block kind: %w", err)
			}
			switch kind {
			case protocol.UpdateOutOfRangeObjects:
				outOfRange, err := reader.ReadU32()
				if err != nil {
					return 0, err
				}
				for index := uint32(0); index < outOfRange; index++ {
					if _, err := reader.ReadPackedGUID(); err != nil {
						return 0, err
					}
				}
			case protocol.UpdateCreateObject, protocol.UpdateCreateObject2:
				guid, typeID, _, err := parseCreateObjectBlock(reader, 0)
				if err != nil {
					return 0, err
				}
				if guid == playerGUID && typeID == 4 {
					count++
				}
			case protocol.UpdateValues:
				if err := skipValuesUpdate(reader); err != nil {
					return 0, err
				}
			case protocol.UpdateMovement:
				if err := skipMovementUpdate(reader); err != nil {
					return 0, err
				}
			default:
				return 0, fmt.Errorf("unsupported update block kind=%d while counting player create deliveries", kind)
			}
		}
		if reader.Remaining() != 0 {
			return 0, fmt.Errorf("update-object payload has %d trailing bytes", reader.Remaining())
		}
	}
	return count, nil
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
	return skipCreateMovementExtra(reader, flags)
}

func skipCreateMovementExtra(reader *protocol.Buffer, flags uint16) error {
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

func readPlayerCreateMovement(reader *protocol.Buffer, flags uint16) (playerCreateMovement, error) {
	if flags&0x20 == 0 {
		return playerCreateMovement{}, errors.New("player create is missing UPDATEFLAG_LIVING")
	}
	movement, err := readLivingMovement(reader)
	if err != nil {
		return movement, err
	}
	if err := skipCreateMovementExtra(reader, flags); err != nil {
		return movement, err
	}
	return movement, nil
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

type playerCreateMovement struct {
	Flags           uint32
	Flags2          uint16
	Position        [4]float32
	OnTransport     bool
	TransportGUID   uint64
	TransportOffset [4]float32
}

func skipLivingMovement(reader *protocol.Buffer) error {
	_, err := readLivingMovement(reader)
	return err
}

func readLivingMovement(reader *protocol.Buffer) (playerCreateMovement, error) {
	var movement playerCreateMovement
	var err error
	if movement.Flags, err = reader.ReadU32(); err != nil {
		return movement, fmt.Errorf("player movement flags are truncated: %w", err)
	}
	if movement.Flags2, err = reader.ReadU16(); err != nil {
		return movement, fmt.Errorf("player extra movement flags are truncated: %w", err)
	}
	if _, err := reader.ReadU32(); err != nil {
		return movement, fmt.Errorf("player movement time is truncated: %w", err)
	}
	for index := range movement.Position {
		if movement.Position[index], err = reader.ReadF32(); err != nil {
			return movement, fmt.Errorf("player movement position component %d is truncated: %w", index, err)
		}
	}
	if movement.Flags&0x200 != 0 {
		movement.OnTransport = true
		if movement.TransportGUID, err = reader.ReadPackedGUID(); err != nil {
			return movement, fmt.Errorf("player transport GUID is truncated: %w", err)
		}
		for index := range movement.TransportOffset {
			if movement.TransportOffset[index], err = reader.ReadF32(); err != nil {
				return movement, fmt.Errorf("player transport offset component %d is truncated: %w", index, err)
			}
		}
		if _, err := reader.ReadU32(); err != nil {
			return movement, fmt.Errorf("player transport time is truncated: %w", err)
		}
		if _, err := reader.ReadU8(); err != nil {
			return movement, fmt.Errorf("player transport seat is truncated: %w", err)
		}
		if movement.Flags2&0x1 != 0 {
			if _, err := reader.ReadU32(); err != nil {
				return movement, fmt.Errorf("player interpolated transport time is truncated: %w", err)
			}
		}
	}
	if movement.Flags&(0x2000|0x4000) != 0 || movement.Flags2&0x2 != 0 {
		if _, err := reader.ReadF32(); err != nil {
			return movement, fmt.Errorf("player pitch is truncated: %w", err)
		}
	}
	if _, err := reader.ReadU32(); err != nil {
		return movement, fmt.Errorf("player fall time is truncated: %w", err)
	}
	if movement.Flags&0x1000 != 0 {
		if _, err := reader.Read(16); err != nil {
			return movement, fmt.Errorf("player jump movement is truncated: %w", err)
		}
	}
	if movement.Flags&0x04000000 != 0 {
		if _, err := reader.ReadF32(); err != nil {
			return movement, fmt.Errorf("player spline elevation is truncated: %w", err)
		}
	}
	if _, err := reader.Read(36); err != nil {
		return movement, fmt.Errorf("player movement speeds are truncated: %w", err)
	}
	if movement.Flags&0x08000000 != 0 {
		if err := skipCreateSpline(reader); err != nil {
			return movement, fmt.Errorf("player create spline is truncated: %w", err)
		}
	}
	return movement, nil
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

func pairedLoginTrace(trace protocoltrace.Trace, playerGUID uint64) (protocoltrace.Trace, error) {
	filtered := trace
	filtered.Events = nil
	loginFound := false
	prefix := fmt.Sprintf("recipient-guid=%d ", playerGUID)
	for _, event := range trace.Events {
		if event.Direction == protocoltrace.ClientToServer && event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) {
			guid, err := loginPlayerGUID(event)
			if err != nil {
				return protocoltrace.Trace{}, err
			}
			if guid == playerGUID {
				filtered.Events = append(filtered.Events, event)
				loginFound = true
			}
		} else if event.Direction == protocoltrace.ServerToClient && strings.HasPrefix(event.State, prefix) {
			filtered.Events = append(filtered.Events, event)
		}
	}
	if !loginFound {
		return protocoltrace.Trace{}, fmt.Errorf("paired login trace has no request for character %d", playerGUID)
	}
	return filtered, nil
}

func runRealCharacterLoginReplay(workDir string, guid, peerGUID uint64, tracePath string, petCooldownSpell, petPowerSpell, petXPAward, petAuraSourceSpell, petFocusAuraSpell, petFeedSpell uint32, petFoodGUID uint64, lfgDungeonID, instanceEntryMapID, instanceEntryID, statsMinLevel uint32, replayPlayerStartMessage bool, questRewardTwiceID uint32, replayPetCritter, replayFarTeleport bool) error {
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
	cfg.PlayerSaveStatsMinLevel = statsMinLevel
	if replayPlayerStartMessage {
		cfg.PlayerStartString = "logincheck-player-start-message"
	}
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
	if peerGUID != 0 {
		if peerGUID == guid {
			return fmt.Errorf("paired login replay requires distinct character GUIDs")
		}
		var accountID, guildID, mapID int64
		if err := stores.Characters.DB.QueryRowContext(ctx, `SELECT c.account, COALESCE(g.guildid, 0), c.map FROM characters c LEFT JOIN guild_member g ON g.guid = c.guid WHERE c.guid = ?`, guid).Scan(&accountID, &guildID, &mapID); err != nil {
			return fmt.Errorf("read first paired character: %w", err)
		}
		var peerAccountID, peerGuildID, peerMapID int64
		if err := stores.Characters.DB.QueryRowContext(ctx, `SELECT c.account, COALESCE(g.guildid, 0), c.map FROM characters c LEFT JOIN guild_member g ON g.guid = c.guid WHERE c.guid = ?`, peerGUID).Scan(&peerAccountID, &peerGuildID, &peerMapID); err != nil {
			return fmt.Errorf("read second paired character: %w", err)
		}
		if accountID == peerAccountID || guildID == 0 || guildID != peerGuildID || mapID != peerMapID {
			return fmt.Errorf("paired login replay requires different accounts in the same guild and map")
		}
	}
	server := world.NewServer(stores, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg.RealmID, cfg)
	if err := server.Initialize(ctx); err != nil {
		server.Stop()
		return err
	}
	var farTeleportMap uint32
	var farTeleportPosition [4]float32
	if replayFarTeleport {
		var currentMap uint32
		if err := stores.Characters.DB.QueryRowContext(ctx, "SELECT map FROM characters WHERE guid = ?", guid).Scan(&currentMap); err != nil {
			server.Stop()
			return fmt.Errorf("read far-teleport replay source map: %w", err)
		}
		rows, err := stores.World.DB.QueryContext(ctx, "SELECT map, position_x, position_y, position_z, orientation FROM creature WHERE map <> ? ORDER BY map, guid LIMIT 256", currentMap)
		if err != nil {
			server.Stop()
			return fmt.Errorf("select far-teleport world spawn: %w", err)
		}
		found := false
		for rows.Next() {
			var mapID int64
			var position [4]float32
			if rows.Scan(&mapID, &position[0], &position[1], &position[2], &position[3]) != nil || mapID < 0 || mapID > int64(^uint32(0)) {
				continue
			}
			if _, mapFound, mapErr := server.Data.Map(uint32(mapID)); mapErr != nil || !mapFound {
				continue
			}
			valid := true
			for _, coordinate := range position {
				valid = valid && !math.IsNaN(float64(coordinate)) && !math.IsInf(float64(coordinate), 0)
			}
			if !valid {
				continue
			}
			farTeleportMap, farTeleportPosition, found = uint32(mapID), position, true
			break
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			server.Stop()
			return fmt.Errorf("scan far-teleport world spawn: %w", rowsErr)
		}
		if !found {
			server.Stop()
			return fmt.Errorf("no valid creature spawn exists outside source map %d", currentMap)
		}
	}
	if petAuraSourceSpell != 0 {
		if _, found, err := server.Data.Spell(petAuraSourceSpell); err != nil || !found {
			server.Stop()
			if err != nil {
				return fmt.Errorf("load owner pet-aura fixture source spell %d: %w", petAuraSourceSpell, err)
			}
			return fmt.Errorf("owner pet-aura fixture source spell %d is missing from DBC", petAuraSourceSpell)
		}
		if _, err := stores.Characters.DB.ExecContext(ctx, "INSERT OR REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", guid, petAuraSourceSpell); err != nil {
			server.Stop()
			return fmt.Errorf("inject isolated owner pet-aura fixture spell %d: %w", petAuraSourceSpell, err)
		}
	}
	if questRewardTwiceID != 0 {
		if _, err := stores.Characters.DB.ExecContext(ctx, "DELETE FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", guid, questRewardTwiceID); err != nil {
			server.Stop()
			return fmt.Errorf("prepare first quest reward replay state: %w", err)
		}
		if _, err := stores.Characters.DB.ExecContext(ctx, "DELETE FROM character_queststatus WHERE guid = ? AND quest = ?", guid, questRewardTwiceID); err != nil {
			server.Stop()
			return fmt.Errorf("prepare rewarded quest replay status: %w", err)
		}
		if _, err := stores.Characters.DB.ExecContext(ctx, "INSERT INTO character_queststatus (guid, quest, status) VALUES (?, ?, 1)", guid, questRewardTwiceID); err != nil {
			server.Stop()
			return fmt.Errorf("prepare completed quest replay status: %w", err)
		}
	}
	var replayCritterPetID int64
	if replayPetCritter {
		var activePets int64
		if err := stores.Characters.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_pet WHERE owner = ? AND slot = 0", guid).Scan(&activePets); err != nil {
			server.Stop()
			return fmt.Errorf("check active pet before critter replay: %w", err)
		}
		if activePets != 0 {
			server.Stop()
			return fmt.Errorf("critter replay requires a character with no active pet; GUID %d has %d", guid, activePets)
		}
		var entry, model int64
		var petName string
		if err := stores.World.DB.QueryRowContext(ctx, `SELECT entry, COALESCE(NULLIF(modelid1, 0), NULLIF(modelid2, 0), NULLIF(modelid3, 0), NULLIF(modelid4, 0), 0), COALESCE(name, '')
			FROM creature_template WHERE type = 8 AND (modelid1 > 0 OR modelid2 > 0 OR modelid3 > 0 OR modelid4 > 0) ORDER BY entry LIMIT 1`).Scan(&entry, &model, &petName); err != nil || entry == 0 || model == 0 {
			server.Stop()
			return fmt.Errorf("find a creature-template critter fixture: entry=%d model=%d err=%v", entry, model, err)
		}
		var level, petID int64
		if err := stores.Characters.DB.QueryRowContext(ctx, "SELECT level FROM characters WHERE guid = ?", guid).Scan(&level); err != nil {
			server.Stop()
			return fmt.Errorf("read critter replay owner level: %w", err)
		}
		if err := stores.Characters.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM character_pet").Scan(&petID); err != nil || petID <= 0 {
			server.Stop()
			return fmt.Errorf("allocate critter replay pet ID: %d %v", petID, err)
		}
		if petName == "" {
			petName = "Companion"
		}
		if _, err := stores.Characters.DB.ExecContext(ctx, `INSERT INTO character_pet (id, entry, owner, modelid, CreatedBySpell, PetType, level, exp, Reactstate, name, renamed, slot, curhealth, curmana, curhappiness, savetime, abdata)
			VALUES (?, ?, ?, ?, 0, 0, ?, 0, 1, ?, 0, 0, 1, 0, 0, ?, '')`, petID, entry, guid, model, level, petName, time.Now().Unix()); err != nil {
			server.Stop()
			return fmt.Errorf("seed critter replay pet: %w", err)
		}
		replayCritterPetID = petID
		defer func() {
			_, _ = stores.Characters.DB.ExecContext(context.Background(), "DELETE FROM character_pet WHERE id = ? AND owner = ?", replayCritterPetID, guid)
		}()
	}
	before, err := snapshotCharacterState(stores.Characters.DB, stores.World.DB, stores.Auth.DB, guid)
	if err != nil {
		server.Stop()
		return err
	}
	petBefore, err := snapshotActivePetLoginState(ctx, stores.Characters.DB, guid)
	if err != nil {
		server.Stop()
		return err
	}
	var peerBefore map[string]characterTableSnapshot
	if peerGUID != 0 {
		peerBefore, err = snapshotCharacterState(stores.Characters.DB, stores.World.DB, stores.Auth.DB, peerGUID)
		if err != nil {
			server.Stop()
			return err
		}
	}
	var raceID, classID, cinematicBefore uint32
	if err := stores.Characters.DB.QueryRowContext(ctx, "SELECT race, class, COALESCE(cinematic, 0) FROM characters WHERE guid = ?", guid).Scan(&raceID, &classID, &cinematicBefore); err != nil {
		server.Stop()
		return fmt.Errorf("read replay character cinematic state: %w", err)
	}
	var savedTransportGUID int64
	var savedTransportOffset [4]float32
	if err := stores.Characters.DB.QueryRowContext(ctx, "SELECT COALESCE(transguid, 0), COALESCE(trans_x, 0), COALESCE(trans_y, 0), COALESCE(trans_z, 0), COALESCE(trans_o, 0) FROM characters WHERE guid = ?", guid).Scan(&savedTransportGUID, &savedTransportOffset[0], &savedTransportOffset[1], &savedTransportOffset[2], &savedTransportOffset[3]); err != nil {
		server.Stop()
		return fmt.Errorf("read saved transport attachment: %w", err)
	}
	expectedCinematic := sourceLoginCinematicID(server.Data, raceID, classID)
	var trace protocoltrace.Trace
	var replayErr error
	if peerGUID != 0 {
		trace, replayErr = world.ReplayCharacterPairLogin(ctx, server, guid, peerGUID)
	} else if petCooldownSpell != 0 {
		trace, replayErr = world.ReplayCharacterPetCooldown(ctx, server, guid, petCooldownSpell)
	} else if petPowerSpell != 0 {
		trace, replayErr = world.ReplayCharacterPetPower(ctx, server, guid, petPowerSpell)
	} else if petXPAward != 0 {
		trace, replayErr = world.ReplayCharacterPetXP(ctx, server, guid, petXPAward)
	} else if petAuraSourceSpell != 0 {
		trace, replayErr = world.ReplayCharacterPetAura(ctx, server, guid, petAuraSourceSpell)
	} else if petFocusAuraSpell != 0 {
		trace, replayErr = world.ReplayCharacterPetFocusAura(ctx, server, guid, petFocusAuraSpell)
	} else if petFeedSpell != 0 {
		trace, replayErr = world.ReplayCharacterPetFeed(ctx, server, guid, petFeedSpell, petFoodGUID)
	} else if questRewardTwiceID != 0 {
		trace, replayErr = world.ReplayCharacterQuestRewardTwice(ctx, server, guid, questRewardTwiceID)
	} else if replayPetCritter {
		trace, replayErr = world.ReplayCharacterPetCritter(ctx, server, guid)
	} else if lfgDungeonID != 0 {
		trace, replayErr = world.ReplayCharacterLFGTeleport(ctx, server, guid, lfgDungeonID)
	} else if replayFarTeleport {
		trace, replayErr = world.ReplayCharacterFarTeleport(ctx, server, guid, farTeleportMap, farTeleportPosition[0], farTeleportPosition[1], farTeleportPosition[2], farTeleportPosition[3])
	} else if instanceEntryID != 0 {
		trace, replayErr = world.ReplayCharacterInstanceEntry(ctx, server, guid, instanceEntryMapID, instanceEntryID)
	} else {
		trace, replayErr = world.ReplayCharacterLogin(ctx, server, guid)
	}
	cancel()
	server.Stop()
	after, snapshotErr := snapshotCharacterState(stores.Characters.DB, stores.World.DB, stores.Auth.DB, guid)
	var peerAfter map[string]characterTableSnapshot
	var peerSnapshotErr error
	if peerGUID != 0 {
		peerAfter, peerSnapshotErr = snapshotCharacterState(stores.Characters.DB, stores.World.DB, stores.Auth.DB, peerGUID)
	}
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
	if replayPetCritter {
		if err := checkCritterPetLoginTrace(trace); err != nil {
			return fmt.Errorf("critter pet packet replay failed: %w", err)
		}
	}
	if savedTransportGUID != 0 {
		if err := checkAttachedTransportLogin(trace, guid, uint64(savedTransportGUID), savedTransportOffset); err != nil {
			return fmt.Errorf("attached transport login replay failed: %w", err)
		}
	}
	if snapshotErr != nil {
		return snapshotErr
	}
	if peerSnapshotErr != nil {
		return peerSnapshotErr
	}
	var deltaErr error
	if questRewardTwiceID != 0 {
		deltaErr = validateCharacterStateDeltaWithQuestReward(before, after)
	} else if statsMinLevel != 0 {
		deltaErr = validateCharacterStateDeltaWithStats(before, after, petFeedSpell != 0)
	} else {
		deltaErr = validateCharacterStateDelta(before, after, petFeedSpell != 0)
	}
	if deltaErr != nil {
		return fmt.Errorf("real-character state delta mismatch (trace saved): %w", deltaErr)
	}
	if peerGUID == 0 {
		var cinematicAfter uint32
		if err := stores.Characters.DB.QueryRowContext(context.Background(), "SELECT COALESCE(cinematic, 0) FROM characters WHERE guid = ?", guid).Scan(&cinematicAfter); err != nil {
			return fmt.Errorf("read replayed character cinematic state: %w", err)
		}
		if err := checkLoginCinematic(trace, expectedCinematic, cinematicBefore, cinematicAfter); err != nil {
			return err
		}
		if replayPlayerStartMessage {
			if err := checkLoginPlayerStartMessage(trace, guid, cfg.PlayerStartString, cinematicBefore == 0); err != nil {
				return err
			}
		}
	}
	if peerGUID != 0 {
		if err := validateCharacterStateDelta(peerBefore, peerAfter, false); err != nil {
			return fmt.Errorf("second real-character state delta mismatch (trace saved): %w", err)
		}
	}
	if statsMinLevel != 0 {
		var level uint32
		if err := stores.Characters.DB.QueryRowContext(context.Background(), "SELECT level FROM characters WHERE guid = ?", guid).Scan(&level); err != nil {
			return fmt.Errorf("read replayed character level for stats save: %w", err)
		}
		if level < statsMinLevel {
			return fmt.Errorf("stats-save fixture level=%d is below configured minimum=%d", level, statsMinLevel)
		}
		var statsRows int
		if err := stores.Characters.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM character_stats WHERE guid = ?", guid).Scan(&statsRows); err != nil {
			return fmt.Errorf("read replayed character_stats row: %w", err)
		}
		if statsRows != 1 {
			return fmt.Errorf("character_stats rows after qualified logout=%d, want 1", statsRows)
		}
		if err := validateCharacterStatsSpellPower(context.Background(), stores.Characters.DB, stores.World.DB, server.Data, guid, trace); err != nil {
			return err
		}
	}
	beforeCharacter, beforeFound := before["characters"]
	afterCharacter, afterFound := after["characters"]
	if !beforeFound || !afterFound || beforeCharacter.Rows != 1 || afterCharacter.Rows != 1 {
		return fmt.Errorf("real-character login replay changed character-row presence before=%t after=%t", beforeFound && beforeCharacter.Rows == 1, afterFound && afterCharacter.Rows == 1)
	}
	loginStarts := make([]int, 0, 2)
	for index, event := range trace.Events {
		if event.Direction == protocoltrace.ClientToServer && event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) {
			loginStarts = append(loginStarts, index)
		}
	}
	wantLogins := 1
	if peerGUID != 0 {
		wantLogins = 2
	}
	if len(loginStarts) != wantLogins {
		return fmt.Errorf("real-character login replay captured %d player-login requests, want %d", len(loginStarts), wantLogins)
	}
	if peerGUID == 0 {
		if err := checkLogin(trace, loginStarts[0]); err != nil {
			return fmt.Errorf("real-character login packet replay failed at event %d: %w", loginStarts[0], err)
		}
	} else {
		firstTrace, err := pairedLoginTrace(trace, guid)
		if err != nil {
			return err
		}
		secondTrace, err := pairedLoginTrace(trace, peerGUID)
		if err != nil {
			return err
		}
		if err := checkLogin(firstTrace, 0); err != nil {
			return fmt.Errorf("first paired login packet replay failed: %w", err)
		}
		if err := checkLogin(secondTrace, 0); err != nil {
			return fmt.Errorf("second paired login packet replay failed: %w", err)
		}
		firstSeesSecond, err := countPlayerCreateBlocks(firstTrace, peerGUID)
		if err != nil {
			return fmt.Errorf("count first recipient's peer create updates: %w", err)
		}
		secondSeesFirst, err := countPlayerCreateBlocks(secondTrace, guid)
		if err != nil {
			return fmt.Errorf("count second recipient's peer create updates: %w", err)
		}
		secondSelfCreates, err := countPlayerCreateBlocks(secondTrace, peerGUID)
		if err != nil {
			return fmt.Errorf("count second recipient's self create updates: %w", err)
		}
		if firstSeesSecond == 0 || secondSeesFirst == 0 || secondSelfCreates == 0 {
			return fmt.Errorf("paired create deliveries missing: first-sees-second=%d second-sees-first=%d second-self=%d", firstSeesSecond, secondSeesFirst, secondSelfCreates)
		}
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
	if peerGUID != 0 {
		fmt.Printf("real-character two-session login replay passed first=%d peer=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", guid, peerGUID, len(trace.Events)-2, len(changed), strings.Join(changed, ","), tracePath)
	} else if statsMinLevel != 0 {
		fmt.Printf("real-character stats save replay passed minimum_level=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", statsMinLevel, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	} else if petCooldownSpell != 0 {
		fmt.Printf("real-character pet cooldown replay passed spell=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", petCooldownSpell, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	} else if petPowerSpell != 0 {
		fmt.Printf("real-character pet power replay passed spell=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", petPowerSpell, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	} else if petXPAward != 0 {
		fmt.Printf("real-character pet XP replay passed award=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", petXPAward, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	} else if petAuraSourceSpell != 0 {
		fmt.Printf("real-character owner pet-aura replay passed source=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", petAuraSourceSpell, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	} else if petFocusAuraSpell != 0 {
		fmt.Printf("real-character pet Focus aura replay passed spell=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", petFocusAuraSpell, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
	} else if petFeedSpell != 0 {
		fmt.Printf("real-character pet feed replay passed spell=%d lua=disabled packets=%d changed_tables=%d diff=%s trace=%s\n", petFeedSpell, len(trace.Events)-1, len(changed), strings.Join(changed, ","), tracePath)
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
	Rows                 int
	Digest               string
	Columns              map[string]string
	PetSpellCooldowns    map[petSpellCooldownKey]petSpellCooldownSnapshot
	InventoryRows        map[string]inventoryRowSnapshot
	AccountInstanceTimes map[uint32]int64
}

type petSpellCooldownKey struct {
	GUID  int64
	Spell int64
}

type petSpellCooldownSnapshot struct {
	Time        int64
	CategoryID  int64
	CategoryEnd int64
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

func validatePetSpellCooldownDelta(before, after map[petSpellCooldownKey]petSpellCooldownSnapshot, now int64) error {
	for key, old := range before {
		current, exists := after[key]
		if exists {
			if current != old {
				return fmt.Errorf("pet cooldown row guid=%d spell=%d was modified", key.GUID, key.Spell)
			}
			continue
		}
		if old.Time >= now {
			return fmt.Errorf("active pet cooldown row guid=%d spell=%d was removed", key.GUID, key.Spell)
		}
	}
	for key := range after {
		if _, exists := before[key]; !exists {
			return fmt.Errorf("new pet cooldown row guid=%d spell=%d was created", key.GUID, key.Spell)
		}
	}
	return nil
}

func validateCharacterStateDelta(before, after map[string]characterTableSnapshot, allowPetFeedProgress bool) error {
	return validateCharacterStateDeltaOptions(before, after, allowPetFeedProgress, false, false)
}

func validateCharacterStateDeltaWithStats(before, after map[string]characterTableSnapshot, allowPetFeedProgress bool) error {
	return validateCharacterStateDeltaOptions(before, after, allowPetFeedProgress, true, false)
}

func validateCharacterStateDeltaWithQuestReward(before, after map[string]characterTableSnapshot) error {
	return validateCharacterStateDeltaOptions(before, after, false, false, true)
}

func validateCharacterStateDeltaOptions(before, after map[string]characterTableSnapshot, allowPetFeedProgress, allowCharacterStats, allowQuestReward bool) error {
	allowedColumns := map[string]map[string]struct{}{
		"characters":                     {"cinematic": {}, "exploredZones": {}, "orientation": {}, "position_x": {}, "position_y": {}, "position_z": {}, "instance_id": {}, "instance_mode_mask": {}, "totaltime": {}, "leveltime": {}, "logout_time": {}, "is_logout_resting": {}},
		"character_achievement":          {"guid": {}, "achievement": {}, "date": {}},
		"character_achievement_progress": {"guid": {}, "criteria": {}, "counter": {}, "date": {}},
		"character_aura":                 {"guid": {}, "casterGuid": {}, "itemGuid": {}, "spell": {}, "effectMask": {}, "recalculateMask": {}, "stackCount": {}, "amount0": {}, "amount1": {}, "amount2": {}, "base_amount0": {}, "base_amount1": {}, "base_amount2": {}, "maxDuration": {}, "remainTime": {}, "remainCharges": {}, "critChance": {}, "applyResilience": {}},
		"character_battleground_data":    {"guid": {}, "instanceId": {}, "team": {}, "joinX": {}, "joinY": {}, "joinZ": {}, "joinO": {}, "joinMapId": {}, "taxiStart": {}, "taxiEnd": {}, "mountSpell": {}},
		"account_instance_times":         {"accountId": {}, "instanceId": {}, "releaseTime": {}},
		"character_pet":                  {"curhealth": {}, "curmana": {}, "exp": {}, "level": {}, "savetime": {}, "slot": {}, "Reactstate": {}},
		"pet_aura":                       {"guid": {}, "casterGuid": {}, "spell": {}, "effectMask": {}, "recalculateMask": {}, "stackCount": {}, "amount0": {}, "amount1": {}, "amount2": {}, "base_amount0": {}, "base_amount1": {}, "base_amount2": {}, "maxDuration": {}, "remainTime": {}, "remainCharges": {}, "critChance": {}, "applyResilience": {}},
		"pet_spell":                      {"active": {}, "guid": {}, "spell": {}},
		"pet_spell_cooldown":             {"guid": {}, "spell": {}, "time": {}, "categoryId": {}, "categoryEnd": {}},
	}
	allowedColumns["characters"]["health"] = struct{}{}
	allowedColumns["characters"]["equipmentCache"] = struct{}{}
	allowedColumns["characters"]["extra_flags"] = struct{}{}
	allowedColumns["characters"]["knownTitles"] = struct{}{}
	allowedColumns["characters"]["at_login"] = struct{}{}
	allowedColumns["characters"]["latency"] = struct{}{}
	allowedColumns["characters"]["map"] = struct{}{}
	allowedColumns["characters"]["playerFlags"] = struct{}{}
	allowedColumns["characters"]["rest_bonus"] = struct{}{}
	allowedColumns["characters"]["taximask"] = struct{}{}
	for _, column := range []string{"trans_x", "trans_y", "trans_z", "trans_o", "transguid"} {
		allowedColumns["characters"][column] = struct{}{}
	}
	allowedColumns["characters"]["zone"] = struct{}{}
	for power := 1; power <= 7; power++ {
		allowedColumns["characters"][fmt.Sprintf("power%d", power)] = struct{}{}
	}
	allowedColumns["account_characters"] = map[string]struct{}{"online": {}}
	allowedColumns["auth_account_online"] = map[string]struct{}{"online": {}}
	allowedColumns["character_spell"] = map[string]struct{}{"guid": {}, "spell": {}, "active": {}, "disabled": {}}
	allowedColumns["character_skills"] = map[string]struct{}{"guid": {}, "skill": {}, "value": {}, "max": {}}
	allowedColumns["character_fishingsteps"] = map[string]struct{}{"guid": {}, "fishingSteps": {}}
	allowedColumns["character_spell_cooldown"] = map[string]struct{}{"guid": {}, "spell": {}, "item": {}, "time": {}, "categoryId": {}, "categoryEnd": {}}
	allowedColumns["character_inventory"] = map[string]struct{}{"guid": {}, "bag": {}, "slot": {}, "item": {}}
	allowedColumns["inventory_item_instances"] = map[string]struct{}{"guid": {}, "itemEntry": {}, "owner_guid": {}, "creatorGuid": {}, "giftCreatorGuid": {}, "count": {}, "duration": {}, "charges": {}, "flags": {}, "enchantments": {}, "randomPropertyId": {}, "durability": {}, "playedTime": {}, "text": {}}
	allowedRowChanges := map[string]bool{"character_achievement": true, "character_achievement_progress": true, "character_aura": true, "character_battleground_data": true, "character_fishingsteps": true, "character_inventory": true, "character_spell_cooldown": true, "inventory_item_instances": true, "pet_aura": true, "pet_spell": true, "pet_spell_cooldown": true, "account_instance_times": true}
	if allowQuestReward {
		allowedColumns["characters"]["xp"] = struct{}{}
		allowedColumns["characters"]["money"] = struct{}{}
		allowedColumns["characters"]["level"] = struct{}{}
		allowedColumns["character_queststatus"] = map[string]struct{}{"guid": {}, "quest": {}, "status": {}, "explored": {}, "timer": {}, "mobcount1": {}, "mobcount2": {}, "mobcount3": {}, "mobcount4": {}, "itemcount1": {}, "itemcount2": {}, "itemcount3": {}, "itemcount4": {}, "itemcount5": {}, "itemcount6": {}, "playercount": {}}
		allowedColumns["character_queststatus_rewarded"] = map[string]struct{}{"guid": {}, "quest": {}, "active": {}}
		allowedRowChanges["character_queststatus"] = true
		allowedRowChanges["character_queststatus_rewarded"] = true
	}
	if allowCharacterStats {
		statsColumns := map[string]struct{}{"guid": {}, "maxhealth": {}, "strength": {}, "agility": {}, "stamina": {}, "intellect": {}, "spirit": {}, "armor": {}, "resHoly": {}, "resFire": {}, "resNature": {}, "resFrost": {}, "resShadow": {}, "resArcane": {}, "blockPct": {}, "dodgePct": {}, "parryPct": {}, "critPct": {}, "rangedCritPct": {}, "spellCritPct": {}, "attackPower": {}, "rangedAttackPower": {}, "spellPower": {}, "resilience": {}}
		for power := 1; power <= 7; power++ {
			statsColumns[fmt.Sprintf("maxpower%d", power)] = struct{}{}
		}
		allowedColumns["character_stats"] = statsColumns
		allowedRowChanges["character_stats"] = true
	}
	allowedRowChanges["character_spell"] = true
	allowedRowChanges["character_skills"] = true
	for table, beforeTable := range before {
		afterTable, exists := after[table]
		if !exists {
			return fmt.Errorf("snapshot table %s disappeared", table)
		}
		changedColumns := changedCharacterColumns(beforeTable, afterTable)
		rowsChanged := beforeTable.Rows != afterTable.Rows
		digestChanged := beforeTable.Digest != afterTable.Digest
		if table == "character_battleground_data" && afterTable.Rows != 1 {
			return fmt.Errorf("battleground return-state save left %d rows, want exactly one", afterTable.Rows)
		}
		if table == "character_stats" && allowCharacterStats && afterTable.Rows != 1 {
			return fmt.Errorf("character stats save left %d rows, want exactly one", afterTable.Rows)
		}
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
			if table == "character_pet" && column == "curhappiness" && (allowPetFeedProgress || sort.SearchStrings(changedColumns, "savetime") < len(changedColumns) && changedColumns[sort.SearchStrings(changedColumns, "savetime")] == "savetime") {
				continue
			}
			if _, exists := allowed[column]; !exists {
				return fmt.Errorf("unclassified state mutation table=%s column=%s", table, column)
			}
		}
		if table == "pet_spell_cooldown" {
			if err := validatePetSpellCooldownDelta(beforeTable.PetSpellCooldowns, afterTable.PetSpellCooldowns, time.Now().Unix()); err != nil {
				return err
			}
		}
		if table == "account_instance_times" {
			if err := validateAccountInstanceTimeDelta(beforeTable.AccountInstanceTimes, afterTable.AccountInstanceTimes, time.Now().Unix()); err != nil {
				return err
			}
		}
	}
	for table := range after {
		if _, exists := before[table]; !exists {
			return fmt.Errorf("snapshot table %s appeared", table)
		}
	}
	if err := validateInventoryStateDelta(before, after, allowPetFeedProgress); err != nil {
		return err
	}
	return nil
}

func snapshotCharacterState(db, worldDB, authDB *sql.DB, guid uint64) (map[string]characterTableSnapshot, error) {
	var accountID int64
	if err := db.QueryRow("SELECT account FROM characters WHERE guid = ?", guid).Scan(&accountID); err != nil {
		return nil, fmt.Errorf("snapshot account for character %d: %w", guid, err)
	}
	authAccount, err := snapshotAuthAccountOnline(authDB, accountID)
	if err != nil {
		return nil, err
	}
	queries := []struct{ name, sql string }{
		{"characters", "SELECT * FROM characters WHERE guid = ?"},
		{"account_characters", "SELECT guid, online FROM characters WHERE account = (SELECT account FROM characters WHERE guid = ?) ORDER BY guid"},
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
	result["auth_account_online"] = authAccount
	inventoryRows := map[string]map[string]inventoryRowSnapshot{"character_inventory": {}, "inventory_item_instances": {}}
	itemTemplateExists := make(map[int64]bool)
	instanceTimes := make(map[uint32]int64)
	defer func() {
		for table, rows := range inventoryRows {
			snapshot := result[table]
			snapshot.InventoryRows = rows
			result[table] = snapshot
		}
		snapshot := result["account_instance_times"]
		snapshot.AccountInstanceTimes = instanceTimes
		result["account_instance_times"] = snapshot
	}()
	for _, query := range queries {
		if query.name == "character_inventory" || query.name == "inventory_item_instances" {
			inventoryRows[query.name] = make(map[string]inventoryRowSnapshot)
		}
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
		petCooldowns := make(map[petSpellCooldownKey]petSpellCooldownSnapshot)
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
			if key, row, ok := captureInventoryRow(query.name, columns, values); ok {
				if query.name == "inventory_item_instances" && row.ItemEntry > 0 {
					if exists, cached := itemTemplateExists[row.ItemEntry]; cached {
						row.ItemTemplateValid = exists
					} else {
						if worldDB == nil {
							rows.Close()
							return nil, fmt.Errorf("inventory item-template snapshot requires the world database")
						}
						var found int64
						templateErr := worldDB.QueryRow("SELECT 1 FROM item_template WHERE entry = ?", row.ItemEntry).Scan(&found)
						if templateErr == nil {
							row.ItemTemplateValid = true
						} else if templateErr != sql.ErrNoRows {
							rows.Close()
							return nil, fmt.Errorf("inventory item-template snapshot entry %d: %w", row.ItemEntry, templateErr)
						}
						itemTemplateExists[row.ItemEntry] = row.ItemTemplateValid
					}
				}
				inventoryRows[query.name][key] = row
			}
			if query.name == "account_instance_times" {
				var instanceID uint32
				var releaseTime int64
				var accountID uint32
				for index, column := range columns {
					switch column {
					case "accountId":
						accountID = uint32(snapshotInt64(values[index]))
					case "instanceId":
						instanceID = uint32(snapshotInt64(values[index]))
					case "releaseTime":
						releaseTime = snapshotInt64(values[index])
					}
				}
				if accountID > 0 && instanceID > 0 {
					instanceTimes[instanceID] = releaseTime
				}
			}
			if query.name == "pet_spell_cooldown" {
				var key petSpellCooldownKey
				var cooldown petSpellCooldownSnapshot
				for index, column := range columns {
					switch column {
					case "guid":
						key.GUID = snapshotInt64(values[index])
					case "spell":
						key.Spell = snapshotInt64(values[index])
					case "time":
						cooldown.Time = snapshotInt64(values[index])
					case "categoryId":
						cooldown.CategoryID = snapshotInt64(values[index])
					case "categoryEnd":
						cooldown.CategoryEnd = snapshotInt64(values[index])
					}
				}
				if key.GUID != 0 && key.Spell != 0 {
					petCooldowns[key] = cooldown
				}
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
		result[query.name] = characterTableSnapshot{Rows: len(rowHashes), Digest: hex.EncodeToString(digest[:]), Columns: columnDigests, PetSpellCooldowns: petCooldowns}
	}
	return result, nil
}

func snapshotInt64(value any) int64 {
	switch number := value.(type) {
	case int64:
		return number
	case int:
		return int64(number)
	case float64:
		return int64(number)
	case []byte:
		var parsed int64
		_, _ = fmt.Sscan(string(number), &parsed)
		return parsed
	case string:
		var parsed int64
		_, _ = fmt.Sscan(number, &parsed)
		return parsed
	default:
		return 0
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
