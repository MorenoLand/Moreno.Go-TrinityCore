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
// (pet_progression.go), mirroring Pet::GivePetLevel. create is
// documented-blocked: it needs the selected live creature
// (ChatHandler::getSelectedCreature; command selections resolve only to
// online players in the Go tree) and the taming pipeline
// (CreateTamedPetFrom / DespawnOrUnsummon / InitTalentForLevel /
// SavePetToDB / SetMinion), none of which has a Go bridge.

// handleCmdPet dispatches the pet sub-table (cs_pet.cpp:61-75) with Trinity
// per-level prefix matching. A bare ".pet" prints the syntax line with no
// permission gate: the root's own 479 is dead in C++ (ChatCommand.h:262
// deprecated overload).
func (s *session) handleCmdPet(ctx context.Context, args []string) {
	const syntax = "Syntax: .pet create|learn|unlearn|level <args>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("create", sub):
		s.handlePetCreateCommand(ctx)
	case strings.HasPrefix("learn", sub):
		s.handlePetLearnCommand(ctx, rest)
	case strings.HasPrefix("unlearn", sub):
		s.handlePetUnlearnCommand(ctx, rest)
	case strings.HasPrefix("level", sub):
		s.handlePetLevelCommand(ctx, rest)
	default:
		s.sendSysMessage(syntax)
	}
}

// petTargetOrSelf mirrors getSelectedPlayerOrSelf (Chat.cpp:344-360) for the
// pet commands (cs_pet.cpp:38-48): the selected online player, else the
// invoker. An unresolvable selection falls back to the invoker silently,
// never nil in-session (same bug class the modify port fixed in dc89b37).
// The C++ also accepts a selected pet unit directly; pet units have no Go
// bridge (documented gap).
func (s *session) petTargetOrSelf() (*session, bool) {
	target := s
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			target = ts
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

// handlePetCreateCommand is documented-blocked (cs_pet.cpp:78-121): taming
// the selected creature needs getSelectedCreature plus the taming pipeline
// (CreatureTemplate family gate, CreateTamedPetFrom, DespawnOrUnsummon,
// InitTalentForLevel, SavePetToDB, SetMinion, PetSpellInitialize).
func (s *session) handlePetCreateCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandPetCreate) {
		return
	}
	s.sendSysMessage("pet create is not supported: selected-unit creature targets and the taming pipeline (CreateTamedPetFrom/DespawnOrUnsummon/InitTalentForLevel/SavePetToDB/SetMinion) have no Go bridge.")
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

// handlePetLearnCommand mirrors HandlePetLearnCommand (cs_pet.cpp:123-158)
// for the supported legs: the spell id is parsed id-only (the C++ also
// accepts |Hspell| links; the learn port documents the same gap), spell
// existence is checked against the DBC, and the spell is taught via the
// pet_spell table with SMSG_PET_LEARNED_SPELL, mirroring
// Pet::learnSpell. SpellMgr::IsSpellValid has no Go model, so DBC-loaded
// spells are assumed structurally valid (same documented gap as the
// cast/learn ports, so the LANG_COMMAND_SPELL_BROKEN branch is
// unreachable).
func (s *session) handlePetLearnCommand(ctx context.Context, args []string) {
	const syntax = "Syntax: .pet learn <spellid>"
	if s.miscDeny(ctx, permissionCommandPetLearn) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	ts, petID, motion, ok := s.petCommandLearnTarget()
	if !ok {
		return
	}
	spellID, ok := s.learnParseSpellID(args[0])
	if !ok {
		return
	}
	if ts.petKnowsSpell(ctx, motion, spellID) {
		s.sendSysMessage(fmt.Sprintf("Pet already has spell: %d", spellID))
		return
	}
	db := s.server.CharactersStore.DB
	if db == nil {
		return
	}
	if _, err := db.ExecContext(ctx, "INSERT OR REPLACE INTO pet_spell (guid, spell, active) VALUES (?, ?, 1)", petID, spellID); err != nil {
		return
	}
	learnBuf := protocol.NewBuffer(4)
	learnBuf.WriteU32(spellID)
	_ = ts.write(uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL), learnBuf.Bytes(), true)
	if motion != nil {
		ts.sendPetSpells(ctx, petID, motion.Entry, motion.ReactState)
	}
	s.sendSysMessage(fmt.Sprintf("Pet has learned spell %d", spellID))
}

// handlePetUnlearnCommand mirrors HandlePetUnlearnCommand
// (cs_pet.cpp:160-182): a known spell is dropped from the pet_spell table
// with SMSG_PET_UNLEARNED_SPELL, mirroring Pet::removeSpell.
func (s *session) handlePetUnlearnCommand(ctx context.Context, args []string) {
	const syntax = "Syntax: .pet unlearn <spellid>"
	if s.miscDeny(ctx, permissionCommandPetUnlearn) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	ts, petID, motion, ok := s.petCommandLearnTarget()
	if !ok {
		return
	}
	spellID, ok := s.learnParseSpellID(args[0])
	if !ok {
		return
	}
	if !ts.petKnowsSpell(ctx, motion, spellID) {
		s.sendSysMessage("Pet doesn't have that spell")
		return
	}
	db := s.server.CharactersStore.DB
	if db == nil {
		return
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ? AND spell = ?", petID, spellID); err != nil {
		return
	}
	unlearnBuf := protocol.NewBuffer(4)
	unlearnBuf.WriteU32(spellID)
	_ = ts.write(uint16(protocol.OpcodeSMSG_PET_UNLEARNED_SPELL), unlearnBuf.Bytes(), true)
	if motion != nil {
		ts.sendPetSpells(ctx, petID, motion.Entry, motion.ReactState)
	}
}

// handlePetLevelCommand mirrors HandlePetLevelCommand (cs_pet.cpp:184-211):
// the delta defaults to ownerLevel - petLevel when no argument is given,
// is bounded by +-STRONG_MAX_LEVEL (80) with LANG_BAD_VALUE (115) on
// violation, and the result is clamped to [1, ownerLevel]. The level change
// itself rides applyPetLevel (pet_progression.go), mirroring
// Pet::GivePetLevel (XP reset to 0, next-level XP from the XP curve).
// InitTalentForLevel has no Go bridge (documented gap, see commands.go).
func (s *session) handlePetLevelCommand(ctx context.Context, args []string) {
	const syntax = "Syntax: .pet level [<leveldelta>]"
	if s.miscDeny(ctx, permissionCommandPetLevel) {
		return
	}
	ts, ok := s.petTargetOrSelf()
	if !ok {
		return
	}
	if ts.player.PetGUID == 0 || ts.player.PetNumber == 0 {
		s.sendSysMessage("Select a player or their pet.") // LANG_SELECT_PLAYER_OR_PET 11016
		return
	}
	motion := s.petCommandMotion(ts)
	if motion == nil {
		s.sendSysMessage("pet level is not supported: the pet must be currently active to change its level.")
		return
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
		s.sendSysMessage("Wrong command value.") // LANG_BAD_VALUE 115
		return
	}
	newLevel := int(petLevel) + delta
	if newLevel < 1 {
		newLevel = 1
	} else if uint32(newLevel) > ownerLevel {
		newLevel = int(ownerLevel)
	}
	if uint32(newLevel) == petLevel {
		return
	}
	nextLevelXP := uint32(0)
	if s.server != nil {
		nextLevelXP = s.server.xpForLevel(ctx, uint32(newLevel)+1) / 20
	}
	if !ts.applyPetLevel(ctx, ts.player.PetNumber, motion.Entry, motion.PetType, petLevel, uint32(newLevel), 0, nextLevelXP) {
		s.sendSysMessage("pet level change failed.")
	}
}

// strongMaxLevel mirrors STRONG_MAX_LEVEL (SharedDefines.h), the delta
// bound in HandlePetLevelCommand (cs_pet.cpp:196).
const strongMaxLevel = 80
