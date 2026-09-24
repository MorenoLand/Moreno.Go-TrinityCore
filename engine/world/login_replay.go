package world

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

func ReplayCharacterLogin(ctx context.Context, server *Server, guid uint64) (protocoltrace.Trace, error) {
	return replayCharacterLogin(ctx, server, guid, 0, 0, 0)
}

func ReplayCharacterPetCooldown(ctx context.Context, server *Server, guid uint64, spellID uint32) (protocoltrace.Trace, error) {
	if spellID == 0 {
		return protocoltrace.Trace{}, errors.New("pet cooldown replay requires a spell ID")
	}
	return replayCharacterLogin(ctx, server, guid, spellID, 0, 0)
}

func ReplayCharacterPetPower(ctx context.Context, server *Server, guid uint64, spellID uint32) (protocoltrace.Trace, error) {
	if spellID == 0 {
		return protocoltrace.Trace{}, errors.New("pet power replay requires a spell ID")
	}
	return replayCharacterLogin(ctx, server, guid, 0, spellID, 0)
}

func ReplayCharacterPetXP(ctx context.Context, server *Server, guid uint64, earnedXP uint32) (protocoltrace.Trace, error) {
	if earnedXP == 0 {
		return protocoltrace.Trace{}, errors.New("pet XP replay requires an XP award")
	}
	return replayCharacterLogin(ctx, server, guid, 0, 0, earnedXP)
}

func replayCharacterLogin(ctx context.Context, server *Server, guid uint64, petCooldownSpell, petPowerSpell, petXPAward uint32) (protocoltrace.Trace, error) {
	if server == nil || server.CharactersStore == nil || server.CharactersStore.DB == nil || guid == 0 {
		return protocoltrace.Trace{}, errors.New("login replay requires a server and character database")
	}
	server.sessionsMu.RLock()
	activeSessions := len(server.sessions)
	server.sessionsMu.RUnlock()
	if activeSessions != 0 || server.TraceRecorder != nil {
		return protocoltrace.Trace{}, errors.New("login replay requires an isolated server instance")
	}
	var accountID int64
	if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", guid).Scan(&accountID); err != nil || accountID <= 0 || accountID > int64(^uint32(0)) {
		return protocoltrace.Trace{}, errors.New("login replay character was not found")
	}
	session := &session{server: server, authed: true, accountID: uint32(accountID), playerGUID: guid, legitimate: map[uint64]struct{}{guid: {}}, characterNames: make(map[uint64]enumCharacter), auras: make(map[uint32]struct{}), auraSlots: make(map[uint32]uint8), channels: make(map[string]struct{}), scale: 1, breathTimer: -1, fatigueTimer: -1, schoolLockouts: make(map[uint32]int64)}
	if server.AuthStore != nil && server.AuthStore.DB != nil {
		var security int64
		if server.AuthStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(gmlevel), 0) FROM account_access WHERE id = ? AND RealmID IN (?, -1)", accountID, server.RealmID).Scan(&security) == nil && security > 0 && security <= 255 {
			session.security = uint8(security)
		}
	}
	recorder := protocoltrace.NewRecorder("morenocore-in-process-login-replay")
	server.TraceRecorder = recorder
	defer func() { server.TraceRecorder = nil }()
	server.sessionsMu.Lock()
	server.sessions[session] = struct{}{}
	server.sessionsMu.Unlock()
	defer func() {
		server.sessionsMu.Lock()
		delete(server.sessions, session)
		server.sessionsMu.Unlock()
	}()
	packet := protocol.NewBuffer(8)
	packet.WriteU64(guid)
	recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_PLAYER_LOGIN), packet.Bytes(), "isolated-character-login")
	if !session.handlePlayerLogin(ctx, packet.Bytes()) {
		return recorder.Snapshot(), errors.New("character login handler rejected the replay")
	}
	if petCooldownSpell != 0 {
		if session.player == nil || session.player.PetGUID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay requires an active pet")
		}
		motion, motionFound := session.petMotionForCast(session.player.PetGUID)
		if !motionFound || !session.petKnowsSpell(ctx, motion, petCooldownSpell) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay pet motion or known spell was not restored")
		}
		spell, spellFound, spellErr := server.Data.Spell(petCooldownSpell)
		if spellErr != nil || !spellFound || spell.Attributes&spellAttributePassive != 0 || motion.GUID != session.player.PetGUID || isHarmfulSpell(spell) && !isSelfCastOnly(spell) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay spell cannot target the current pet")
		}
		categoryID, _, categoryErr := session.spellCooldownCategory(petCooldownSpell)
		if categoryErr != nil || categoryID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay spell has no DBC category")
		}
		server.motionMu.Lock()
		categoryEnd := motion.SpellCategoryCooldowns[categoryID]
		server.motionMu.Unlock()
		if !categoryEnd.After(time.Now()) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet category cooldown was not restored from the database")
		}
		cast := protocol.NewBuffer(24)
		cast.WriteU64(session.player.PetGUID)
		cast.WriteU8(1)
		cast.WriteU32(petCooldownSpell)
		cast.WriteU8(0)
		cast.WriteU32(protocol.SpellTargetFlagUnit)
		cast.WritePackedGUID(session.player.PetGUID)
		target, targetErr := protocol.ReadSpellTargetData(protocol.NewReader(cast.Bytes()[14:]))
		if targetErr != nil || target.UnitGUID != session.player.PetGUID {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay target did not encode the current pet GUID")
		}
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_PET_CAST_SPELL), cast.Bytes(), "pet-category-cooldown-check")
		if !session.handlePetCastSpell(ctx, cast.Bytes()) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown cast handler rejected the replay")
		}
		trace := recorder.Snapshot()
		session.logout()
		if !petCastFailedNotReady(trace, petCooldownSpell) {
			return trace, errors.New("pet cast did not return SPELL_FAILED_NOT_READY for the persisted cooldown")
		}
		return trace, nil
	}
	if petPowerSpell != 0 {
		if session.player == nil || session.player.PetGUID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay requires an active pet")
		}
		motion, motionFound := session.petMotionForCast(session.player.PetGUID)
		spell, spellFound, spellErr := server.Data.Spell(petPowerSpell)
		if !motionFound || spellErr != nil || !spellFound || !session.petKnowsSpell(ctx, motion, petPowerSpell) || spell.PowerType >= 7 || spell.PowerType == petPowerTypeHealth {
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay pet motion, known spell, or supported power type was not restored")
		}
		server.motionMu.Lock()
		_, maximum, _, validPower, _ := petSpellPowerState(motion, spell.PowerType)
		server.motionMu.Unlock()
		cost := ResolvePetSpellPowerCost(spell, maximum, motion.MaxHealth)
		if !validPower || cost == 0 || cost >= maximum {
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay spell has no affordable DBC resource cost")
		}
		server.motionMu.Lock()
		originalPower := motion.Powers[spell.PowerType]
		originalMana := motion.Mana
		originalLastSpell := motion.LastSpell
		originalSpellCooldowns := make(map[uint32]time.Time, len(motion.SpellCooldowns))
		for id, value := range motion.SpellCooldowns {
			originalSpellCooldowns[id] = value
		}
		originalCategoryCooldowns := make(map[uint32]time.Time, len(motion.SpellCategoryCooldowns))
		for id, value := range motion.SpellCategoryCooldowns {
			originalCategoryCooldowns[id] = value
		}
		motion.Powers[spell.PowerType] = cost - 1
		if spell.PowerType == 0 {
			motion.Mana = cost - 1
		}
		server.motionMu.Unlock()
		if session.checkPetSpellPower(motion, spell, 0) {
			server.motionMu.Lock()
			motion.Powers[spell.PowerType], motion.Mana, motion.LastSpell = originalPower, originalMana, originalLastSpell
			motion.SpellCooldowns, motion.SpellCategoryCooldowns = originalSpellCooldowns, originalCategoryCooldowns
			server.motionMu.Unlock()
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay accepted a cast with insufficient resource")
		}
		server.motionMu.Lock()
		motion.Powers[spell.PowerType] = maximum
		if spell.PowerType == 0 {
			motion.Mana = maximum
		}
		server.motionMu.Unlock()
		spell.Effects = [3]wotlk.SpellEffect{}
		target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: motion.GUID}
		if !session.executePetSpell(ctx, motion, spell, 0, target) {
			server.motionMu.Lock()
			motion.Powers[spell.PowerType], motion.Mana, motion.LastSpell = originalPower, originalMana, originalLastSpell
			motion.SpellCooldowns, motion.SpellCategoryCooldowns = originalSpellCooldowns, originalCategoryCooldowns
			server.motionMu.Unlock()
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay spell executor rejected the cast")
		}
		trace := recorder.Snapshot()
		expectedPower := maximum - cost
		server.motionMu.Lock()
		actualPower := motion.Powers[spell.PowerType]
		motion.Powers[spell.PowerType], motion.Mana, motion.LastSpell = originalPower, originalMana, originalLastSpell
		motion.SpellCooldowns, motion.SpellCategoryCooldowns = originalSpellCooldowns, originalCategoryCooldowns
		server.motionMu.Unlock()
		session.logout()
		if actualPower != expectedPower || !petSpellGoPowerMatches(trace, petPowerSpell, motion.GUID, expectedPower) || !petCastFailedWithResult(trace, petPowerSpell, petSpellFailedNoPower) {
			return trace, fmt.Errorf("pet resource replay state=%d want=%d; spell-go power or no-power rejection mismatch", actualPower, expectedPower)
		}
		return trace, nil
	}
	if petXPAward != 0 {
		if session.player == nil || session.player.PetGUID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet XP replay requires an active pet")
		}
		server.motionMu.Lock()
		motion := server.creatureMotion[session.player.PetGUID]
		if motion == nil || motion.PetType != 1 || motion.Health == 0 {
			server.motionMu.Unlock()
			session.logout()
			return recorder.Snapshot(), errors.New("pet XP replay requires a living hunter pet")
		}
		petID, oldLevel, oldXP, currentNextXP := motion.PetID, motion.Level, motion.Experience, motion.PetNextLevelXP
		server.motionMu.Unlock()
		maxLevel := server.Config.MaxPlayerLevel
		if ownerLevel := uint32(session.player.Level); ownerLevel < maxLevel {
			maxLevel = ownerLevel
		}
		xpForLevel := make([]uint32, len(xpCurve))
		copy(xpForLevel, xpCurve[:])
		for level := uint32(1); level < uint32(len(xpForLevel)); level++ {
			xpForLevel[level] = server.xpForLevel(ctx, level)
		}
		expectedLevel, expectedXP, _ := AdvanceHunterPetExperience(oldLevel, oldXP, petXPAward, maxLevel, currentNextXP, xpForLevel)
		eventsBefore := len(recorder.Snapshot().Events)
		session.giveHunterPetXP(ctx, petXPAward)
		trace := recorder.Snapshot()
		server.motionMu.Lock()
		actualLevel, actualXP := motion.Level, motion.Experience
		server.motionMu.Unlock()
		updatedPet := false
		for _, event := range trace.Events[eventsBefore:] {
			if event.Direction == protocoltrace.ServerToClient && event.Opcode == uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) {
				updatedPet = true
				break
			}
		}
		session.logout()
		var savedLevel, savedXP int64
		if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT level, exp FROM character_pet WHERE id = ? AND owner = ?", petID, guid).Scan(&savedLevel, &savedXP); err != nil {
			return recorder.Snapshot(), err
		}
		if expectedLevel == oldLevel && expectedXP == oldXP || actualLevel != expectedLevel || actualXP != expectedXP || uint32(savedLevel) != expectedLevel || uint32(savedXP) != expectedXP || !updatedPet {
			return recorder.Snapshot(), fmt.Errorf("pet XP replay mismatch expected=(%d,%d) actual=(%d,%d) saved=(%d,%d) update=%t", expectedLevel, expectedXP, actualLevel, actualXP, savedLevel, savedXP, updatedPet)
		}
		return recorder.Snapshot(), nil
	}
	trace := recorder.Snapshot()
	session.logout()
	return trace, nil
}

func petCastFailedNotReady(trace protocoltrace.Trace, spellID uint32) bool {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_PET_CAST_FAILED) {
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(event.Payload)
		if err == nil && len(payload) >= 6 && binary.LittleEndian.Uint32(payload[1:5]) == spellID && payload[5] == spellFailedNotReady {
			return true
		}
	}
	return false
}

func petCastFailedWithResult(trace protocoltrace.Trace, spellID uint32, result uint8) bool {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_PET_CAST_FAILED) {
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(event.Payload)
		if err == nil && len(payload) >= 6 && binary.LittleEndian.Uint32(payload[1:5]) == spellID && payload[5] == result {
			return true
		}
	}
	return false
}

func petSpellGoPowerMatches(trace protocoltrace.Trace, spellID uint32, petGUID uint64, expectedPower uint32) bool {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_SPELL_GO) {
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(event.Payload)
		if err != nil {
			continue
		}
		reader := protocol.NewReader(payload)
		caster, err := reader.ReadPackedGUID()
		if err != nil || caster != petGUID {
			continue
		}
		if _, err := reader.ReadPackedGUID(); err != nil {
			continue
		}
		if _, err := reader.ReadU8(); err != nil {
			continue
		}
		id, err := reader.ReadU32()
		if err != nil || id != spellID {
			continue
		}
		flags, err := reader.ReadU32()
		if err != nil || flags&protocol.SpellCastFlagPowerLeftSelf == 0 {
			continue
		}
		if _, err := reader.ReadU32(); err != nil {
			continue
		}
		hitCount, err := reader.ReadU8()
		if err != nil {
			continue
		}
		valid := true
		for range hitCount {
			if _, err := reader.ReadU64(); err != nil {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		missCount, err := reader.ReadU8()
		if err != nil || missCount != 0 {
			continue
		}
		if _, err := protocol.ReadSpellTargetData(reader); err != nil {
			continue
		}
		power, err := reader.ReadU32()
		if err == nil && power == expectedPower {
			return true
		}
	}
	return false
}
