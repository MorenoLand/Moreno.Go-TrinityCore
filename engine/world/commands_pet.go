package world

import (
	"context"
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// pet command port: pet_commandscript (cs_pet.cpp), the "pet" root with 4
// arms (create, learn, unlearn, level). THIRTY-FIRST of 40 Commands groups
// (cs_script_loader.cpp decl 48 / call 94; call order: quest(93) -> pet(94)).
// Trinity checks permission only on the invoker leaf node
// (ChatCommand.cpp:487), so each arm gates exactly its own C++ permission
// (create/learn/unlearn at RBAC.h:348-350, level at RBAC.h:705). The root's
// own RBAC_PERM_COMMAND_PET (RBAC.h:347, 479) is DEAD in C++: the root uses
// the deprecated 6-arg ChatCommandBuilder overload (ChatCommand.h:262),
// which drops the RBACPermissions param and delegates to the sub-only
// constructor — so a bare ".pet" prints the syntax line with no gate ever
// checked (same dead-root-perm pattern as the mmap/modify/npc/quest ports);
// permissionCommandPet stays in permissions.go as documentation only.
//
// learn/unlearn are native via the pet_spell bridge (INSERT OR REPLACE /
// DELETE + SMSG_PET_LEARNED_SPELL / SMSG_PET_UNLEARNED_SPELL, mirroring
// pet->learnSpell/removeSpell), and level is native via applyPetLevel
// (pet_progression.go), mirroring Pet::GivePetLevel. create is native via
// the selected-creature twin (castCommandSelectedCreature) plus the taming
// tail (createTamedPetFromEntry: DespawnOrUnsummon, DB save, SetMinion,
// PetSpellInitialize); InitTalentForLevel stays unmodeled.

// handleCmdPet dispatches the pet sub-table (cs_pet.cpp:61-75) with Trinity
// per-level prefix matching. A bare ".pet" prints the syntax line with no
// permission gate: the root's own 479 is dead in C++ (ChatCommand.h:262
// deprecated overload). The bool reports C++ handler success: the RBAC
// denial returns false silently so dispatchCommand shows the node's help
// listing (ChatCommand.cpp:296-309); paths that print a message return true.
func (s *session) handleCmdPet(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .pet create|learn|unlearn|level <args>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return true
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("create", sub):
		return s.handlePetCreateCommand(ctx)
	case strings.HasPrefix("learn", sub):
		return s.handlePetLearnCommand(ctx, rest)
	case strings.HasPrefix("unlearn", sub):
		return s.handlePetUnlearnCommand(ctx, rest)
	case strings.HasPrefix("level", sub):
		return s.handlePetLevelCommand(ctx, rest)
	default:
		s.sendSysMessage(syntax)
	}
	return true
}

// petTargetOrSelf mirrors GetSelectedPlayerPetOrOwn (cs_pet.cpp:38-48):
// the selected player's pet, a selected pet unit's owner's pet, or the
// invoker's own pet when nothing is selected. A selected non-pet creature
// returns nullptr in C++ (LANG_SELECT_PLAYER_OR_PET), so it errors here
// instead of falling back to the invoker's pet; a selection that resolves
// to no unit at all (gameobject, stale GUID) falls through to the
// invoker's pet, like getSelectedUnit returning null. Pet units resolve
// via motionIsPet (creature_auras.go), the IsPet twin, and their owner via
// the motion's OwnerGUID.
func (s *session) petTargetOrSelf() (*session, bool) {
	target := s
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			target = ts
		} else if motion := s.castCommandSelectedCreature(); motion != nil {
			if !motionIsPet(motion) {
				s.sendSysMessage("Select a player or their pet.") // LANG_SELECT_PLAYER_OR_PET 11016
				return nil, false
			}
			owner := s.server.playerSessionForGUID(motion.OwnerGUID)
			if owner == nil || owner.player == nil {
				s.sendSysMessage("Select a player or their pet.") // LANG_SELECT_PLAYER_OR_PET 11016
				return nil, false
			}
			target = owner
		}
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
		return nil, false
	}
	return target, true
}

// petCommandMotion returns the target player's active pet motion, or nil.
func (s *session) petCommandMotion(ts *session) *creatureMotion {
	if s.server == nil || ts == nil || ts.player == nil || ts.player.PetGUID == 0 {
		return nil
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	return s.server.findCreatureMotionLocked(ts.player.Map, ts.player.InstanceID, ts.player.PetGUID)
}

// handlePetCreateCommand mirrors HandlePetCreateCommand (cs_pet.cpp:78-121):
// the selected creature (castCommandSelectedCreature twin; a player
// selection resolves to no motion, like getSelectedCreature returning null)
// must not be a pet, its creature_template family must not be NONE, and the
// invoker must not already have a live pet. The original is despawned
// (tame precedent) and the pet is created via createTamedPetFromEntry with
// CreatedBySpell 0. The command level is the C++ command formula — the
// invoker's level, or invoker level - 5 when the creature is below that —
// NOT the tame formula (tame keeps the creature's level floored at
// invoker level - 5). C++ has no class gate here (unlike the hunter-only
// tame spell), and InitTalentForLevel stays unmodeled; the playerLevel < 5
// uint8 underflow wrap in the C++ formula is avoided (Go floors at the
// invoker's level). The level-1-then-level visual (SetUInt32Value(level-1),
// AddToMap, SetUInt32Value(level), cs_pet.cpp:110-116) stays unmodeled:
// createTamedPetFromEntry spawns at the final level.
func (s *session) handlePetCreateCommand(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandPetCreate) {
		return false
	}
	if s.player == nil {
		return true
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil || motion.OwnerGUID != 0 {
		s.sendSysMessage("Select a creature.") // LANG_SELECT_CREATURE
		return true
	}
	entry := motion.Entry
	if entry == 0 {
		entry = uint32((motion.GUID >> 24) & 0x00FFFFFF)
		if entry == 0 {
			entry = uint32(motion.GUID & 0xFFFFFFFF)
		}
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("This creature cannot be tamed.")
		return true
	}
	var family int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT family FROM creature_template WHERE entry = ?", entry).Scan(&family); err != nil {
		s.sendSysMessage("This creature cannot be tamed.")
		return true
	}
	if family == 0 {
		s.sendSysMessage("This creature cannot be tamed. Family id: 0 (CREATURE_FAMILY_NONE).")
		return true
	}
	if s.player.PetGUID != 0 {
		s.sendSysMessage("You already have a pet")
		return true
	}
	playerLevel := uint32(s.player.Level)
	petLevel := playerLevel
	if playerLevel >= 5 && motion.Level < playerLevel-5 {
		petLevel = playerLevel - 5
	}
	targetGUID := motion.GUID
	s.sendDestroyObject(targetGUID, false)
	s.server.motionMu.Lock()
	delete(s.server.motionMapLocked(s.player.Map, s.player.InstanceID), targetGUID)
	s.server.motionMu.Unlock()
	destroyBuf := protocol.NewBuffer(9)
	destroyBuf.WriteU64(targetGUID)
	destroyBuf.WriteU8(0)
	s.server.broadcastToVisibleSetFromSession(s, uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes())
	if s.server.CharactersStore == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	s.createTamedPetFromEntry(ctx, cdb, 0, entry, petLevel)
	return true
}

// petCommandLearnTarget resolves the target for learn/unlearn/level,
// mirroring GetSelectedPlayerPetOrOwn (cs_pet.cpp:38-48): the selected
// player's pet or the invoker's own pet. Missing pet reports
// LANG_SELECT_PLAYER_OR_PET (11016).
func (s *session) petCommandLearnTarget() (*session, uint32, *creatureMotion, bool) {
	ts, ok := s.petTargetOrSelf()
	if !ok {
		return nil, 0, nil, false
	}
	if ts.player.PetGUID == 0 || ts.player.PetNumber == 0 {
		s.sendSysMessage("Select a player or their pet.") // LANG_SELECT_PLAYER_OR_PET 11016
		return nil, 0, nil, false
	}
	return ts, ts.player.PetNumber, s.petCommandMotion(ts), true
}

// petExtractSpellID mirrors ChatHandler::extractSpellIdFromLink
// (Chat.cpp:530-577) over the multi-type extractKeyFromLink (Chat.cpp:398-470):
// a plain token parses with atoul (cAtoi: leading digits, "12abc" -> 12,
// "0x10" -> 0 — NOT the base-0 framework parse the learn arm uses), and a
// |color|Htype:key|h[name]|h|r link resolves by type: Hspell/Henchant/Htrade
// return the key, Htalent returns Talent.dbc SpellRank[rank] (rank from the
// colon param, 0 when absent — the comma form never yields a param, so it
// resolves rank 0 exactly like the C++ strtok walk; rank >= MAX_TALENT_RANK
// (5) or an unknown talent returns 0), Hglyph returns GlyphProperties.dbc
// SpellID for the prop id (unknown prop returns 0). An unknown link type
// returns 0: the C++ sends LANG_WRONG_LINK_TYPE (511) and a null key, and
// the tree convention (modify4/titles ports) is the silent null — the 511
// text is unseeded, not guessed.
func (s *session) petExtractSpellID(arg string) uint32 {
	if !strings.HasPrefix(arg, "|") {
		return uint32(cAtoi(arg))
	}
	segs := strings.Split(arg, "|")
	linkIdx := 1
	if len(segs) > 1 && strings.HasPrefix(segs[1], "c") {
		linkIdx = 2
	}
	if linkIdx >= len(segs) {
		return 0
	}
	parts := strings.SplitN(segs[linkIdx], ":", 2)
	if len(parts) < 2 {
		return 0
	}
	typ, keys := parts[0], parts[1]
	keyParts := strings.Split(keys, ":")
	id := uint32(cAtoi(keyParts[0]))
	param1 := ""
	if len(keyParts) > 1 {
		param1 = keyParts[1]
	}
	switch typ {
	case "Hspell", "Henchant", "Htrade":
		return id
	case "Htalent":
		if s.server == nil || s.server.Data == nil {
			return 0
		}
		rank := 0
		if param1 != "" {
			rank = cAtoi(param1)
		}
		if rank < 0 || rank >= 5 {
			return 0
		}
		entry, found, err := s.server.Data.Talent(id)
		if err != nil || !found {
			return 0
		}
		return entry.SpellRank[rank]
	case "Hglyph":
		if s.server == nil || s.server.Data == nil {
			return 0
		}
		entry, found, err := s.server.Data.GlyphProperties(uint32(cAtoi(param1)))
		if err != nil || !found {
			return 0
		}
		return entry.SpellID
	default:
		return 0
	}
}

// handlePetLearnCommand mirrors HandlePetLearnCommand (cs_pet.cpp:123-158):
// the spell id parses with extractSpellIdFromLink (atoul + link types, see
// petExtractSpellID); a zero id or an unknown spell returns false and the
// framework prints the leaf help — the syntax line here. The pet must
// already know the spell for the "already has" message, and the spell is
// taught via the pet_spell table with SMSG_PET_LEARNED_SPELL, mirroring
// Pet::learnSpell. SpellMgr::IsSpellValid has no Go model, so DBC-loaded
// spells are assumed structurally valid (same documented gap as the
// cast/learn ports, so the LANG_COMMAND_SPELL_BROKEN branch is
// unreachable).
func (s *session) handlePetLearnCommand(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .pet learn <spellid>"
	if !s.commandAllowed(ctx, permissionCommandPetLearn) {
		return false
	}
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return true
	}
	ts, petID, motion, ok := s.petCommandLearnTarget()
	if !ok {
		return true
	}
	spellID := s.petExtractSpellID(args[0])
	if spellID == 0 || s.server == nil || s.server.Data == nil {
		s.sendSysMessage(syntax)
		return true
	}
	if _, found, err := s.server.Data.Spell(spellID); err != nil || !found {
		s.sendSysMessage(syntax)
		return true
	}
	if ts.petKnowsSpell(ctx, motion, spellID) {
		s.sendSysMessage(fmt.Sprintf("Pet already has spell: %d", spellID))
		return true
	}
	db := s.server.CharactersStore.DB
	if db == nil {
		return true
	}
	if _, err := db.ExecContext(ctx, "INSERT OR REPLACE INTO pet_spell (guid, spell, active) VALUES (?, ?, 1)", petID, spellID); err != nil {
		return true
	}
	learnBuf := protocol.NewBuffer(4)
	learnBuf.WriteU32(spellID)
	_ = ts.write(uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL), learnBuf.Bytes(), true)
	if motion != nil {
		ts.sendPetSpells(ctx, petID, motion.Entry, motion.ReactState)
	}
	s.sendSysMessage(fmt.Sprintf("Pet has learned spell %d", spellID))
	return true
}

// handlePetUnlearnCommand mirrors HandlePetUnlearnCommand
// (cs_pet.cpp:160-182): the id parses with extractSpellIdFromLink (see
// petExtractSpellID) and — unlike learn — is never checked against the
// spell DBC: an unknown id simply reports "Pet doesn't have that spell"
// via the HasSpell gate. A known spell is dropped from the pet_spell table
// with SMSG_PET_UNLEARNED_SPELL, mirroring Pet::removeSpell.
func (s *session) handlePetUnlearnCommand(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .pet unlearn <spellid>"
	if !s.commandAllowed(ctx, permissionCommandPetUnlearn) {
		return false
	}
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return true
	}
	ts, petID, motion, ok := s.petCommandLearnTarget()
	if !ok {
		return true
	}
	spellID := s.petExtractSpellID(args[0])
	if !ts.petKnowsSpell(ctx, motion, spellID) {
		s.sendSysMessage("Pet doesn't have that spell")
		return true
	}
	db := s.server.CharactersStore.DB
	if db == nil {
		return true
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ? AND spell = ?", petID, spellID); err != nil {
		return true
	}
	// CharmInfo::RemoveSpellFromActionBar (Unit.cpp:10085): the unlearned
	// spell was the last rank (clear_ab=true, no rank re-learn), so the
	// first bar slot holding any rank of its chain is cleared in place.
	if ts.server != nil {
		if slots, ok := loadPetActionBar(ctx, db, petID, ts.playerGUID); ok {
			first := ts.server.getFirstSpellInChain(spellID)
			for i, slot := range slots {
				if !petSpellActionType(uint8(slot >> 24)) {
					continue
				}
				if action := slot & 0x00FFFFFF; action != 0 && ts.server.getFirstSpellInChain(action) == first {
					slots[i] = makePetActionSlot(petActionPassive, 0) // SetActionBar(i, 0, ACT_PASSIVE)
					savePetActionBar(ctx, db, petID, ts.playerGUID, slots)
					break
				}
			}
		}
	}
	unlearnBuf := protocol.NewBuffer(4)
	unlearnBuf.WriteU32(spellID)
	_ = ts.write(uint16(protocol.OpcodeSMSG_PET_UNLEARNED_SPELL), unlearnBuf.Bytes(), true)
	if motion != nil {
		ts.sendPetSpells(ctx, petID, motion.Entry, motion.ReactState)
	}
	return true
}

// handlePetLevelCommand mirrors HandlePetLevelCommand (cs_pet.cpp:184-211):
// the delta defaults to ownerLevel - petLevel when no argument is given,
// is bounded by +-STRONG_MAX_LEVEL (255) with LANG_BAD_VALUE (115) on
// violation, and the result is clamped to [1, ownerLevel]. The level change
// itself rides applyPetLevel (pet_progression.go), mirroring
// Pet::GivePetLevel (XP reset to 0, next-level XP from the XP curve) —
// including its same-level no-op (Pet.cpp:759-762), so a clamped
// no-change delta returns silently. InitTalentForLevel has no Go bridge
// (documented gap, see commands.go).
func (s *session) handlePetLevelCommand(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .pet level [<leveldelta>]"
	if !s.commandAllowed(ctx, permissionCommandPetLevel) {
		return false
	}
	ts, ok := s.petTargetOrSelf()
	if !ok {
		return true
	}
	if ts.player.PetGUID == 0 || ts.player.PetNumber == 0 {
		s.sendSysMessage("Select a player or their pet.") // LANG_SELECT_PLAYER_OR_PET 11016
		return true
	}
	motion := s.petCommandMotion(ts)
	if motion == nil {
		s.sendSysMessage("pet level is not supported: the pet must be currently active to change its level.")
		return true
	}
	ownerLevel := uint32(ts.player.Level)
	petLevel := motion.Level
	delta := 0
	if len(args) > 0 {
		delta = cAtoi(args[0])
	}
	if delta == 0 {
		delta = int(ownerLevel) - int(petLevel)
	}
	if delta == 0 || delta < -strongMaxLevel || delta > strongMaxLevel {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return true
	}
	newLevel := int(petLevel) + delta
	if newLevel < 1 {
		newLevel = 1
	} else if uint32(newLevel) > ownerLevel {
		newLevel = int(ownerLevel)
	}
	if uint32(newLevel) == petLevel {
		return true
	}
	nextLevelXP := uint32(0)
	if s.server != nil {
		nextLevelXP = s.server.xpForLevel(ctx, uint32(newLevel)+1) / 20
	}
	if !ts.applyPetLevel(ctx, ts.player.PetNumber, motion.Entry, motion.PetType, petLevel, uint32(newLevel), 0, nextLevelXP) {
		s.sendSysMessage("pet level change failed.")
	}
	return true
}

// strongMaxLevel mirrors STRONG_MAX_LEVEL (DBCEnums.h:53), the delta
// bound in HandlePetLevelCommand (cs_pet.cpp:196) and the absolute clamp in
// HandleLevelUpCommand (cs_character.cpp:740).
const strongMaxLevel = 255
