package world

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// This file ports AddSC_learn_commandscript (cs_learn.cpp, whole file is
// 1 column-0 `class` def: 1 x `: public CommandScript` "learn_commandscript").
// Sole-source verified: the class is referenced only by cs_learn.cpp and
// cs_script_loader.cpp (decl 39 / call 84 - TWENTY-FIRST of 39 groups; call
// order re-verified: instance(83) -> learn(84)). Trinity checks permission
// only on the invoker (leaf) node (ChatCommand.cpp:487), so each arm gates
// exactly its own C++ permission.

// learnDebugSpellIDs mirrors the hardcoded list in
// HandleLearnDebugSpellsCommand (cs_learn.cpp:322-330).
var learnDebugSpellIDs = []uint32{63364, 1908, 27680, 62555, 64238, 72525, 66776}

// learnLanguageSpellIDs mirrors lang_description[].spell_id
// (ObjectMgr.cpp:160-184): the non-zero language spell ids learned by
// HandleLearnAllLangCommand (cs_learn.cpp:313-320).
var learnLanguageSpellIDs = []uint32{
	669, 671, 670, 672, 668, 815, 816, 813, 814, 817, 7340, 7341, 17737, 29932,
}

// skillInternalID mirrors SKILL_INTERNAL (SharedDefines.h:3022), the skill
// line id HandleLearnAllGMCommand filters on (cs_learn.cpp:190).
const skillInternalID = 769

// learnTargetOrSelf mirrors ChatHandler::getSelectedPlayerOrSelf
// (cs_learn.cpp:95): the selected online player, else the invoker; an
// unresolvable selection reports LANG_PLAYER_NOT_FOUND (499) per the tree
// convention used by the honor/instance ports.
func (s *session) learnTargetOrSelf() (*session, bool) {
	target := s
	if s.selection != 0 && s.server != nil {
		ts := s.server.playerSessionForGUID(s.selection)
		if ts == nil || ts.player == nil {
			s.sendSysMessage("Player not found.")
			return nil, false
		}
		target = ts
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.")
		return nil, false
	}
	return target, true
}

// learnSelectedPlayer mirrors ChatHandler::getSelectedPlayer
// (cs_learn.cpp:363, 397): the selected player is REQUIRED; a missing
// selection reports LANG_NO_CHAR_SELECTED / LANG_PLAYER_NOT_FOUND.
func (s *session) learnSelectedPlayer(requireMsg string) (*session, bool) {
	if s.selection == 0 || s.server == nil {
		s.sendSysMessage(requireMsg)
		return nil, false
	}
	ts := s.server.playerSessionForGUID(s.selection)
	if ts == nil || ts.player == nil {
		s.sendSysMessage(requireMsg)
		return nil, false
	}
	return ts, true
}

// learnParseSpellID parses the <spell> argument. The C++ SpellInfo parser
// also accepts |Hspell:id|h links; Go commands only accept bare ids (the
// cast port documents the same gap).
func (s *session) learnParseSpellID(arg string) (uint32, bool) {
	id, err := strconv.ParseUint(arg, 10, 32)
	if err != nil || id == 0 {
		s.sendSysMessage("Invalid spell ID.")
		return 0, false
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Spell data is unavailable.")
		return 0, false
	}
	// SpellMgr::IsSpellValid has no Go model; DBC-loaded spells are assumed
	// structurally valid, so the LANG_COMMAND_SPELL_BROKEN branch is
	// unreachable (same documented gap as the cast port).
	if _, found, err := s.server.Data.Spell(uint32(id)); err != nil || !found {
		s.sendSysMessage("There is no such spell.")
		return 0, false
	}
	return uint32(id), true
}

// learnParseAllRanks mirrors Optional<EXACT_SEQUENCE("all")>: the optional
// second argument must be a prefix of "all" (Trinity per-level prefix
// matching), anything else is a syntax error.
func (s *session) learnParseAllRanks(args []string) (bool, bool) {
	if len(args) < 2 {
		return false, true
	}
	if !strings.HasPrefix("all", strings.ToLower(args[1])) {
		return false, false
	}
	return true, true
}

// unlearnSpell mirrors Player::RemoveSpell for the command path: drop the
// spell row, strip auras, notify the client. The talent-row cleanup and the
// disabled-spell variants of RemoveSpell have no Go bridge (documented gap).
func (s *session) unlearnSpell(ctx context.Context, spellID uint32) {
	if s.player == nil || !s.hasLearnedSpell(spellID) {
		return
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_spell WHERE guid = ? AND spell = ?", s.playerGUID, spellID)
	}
	if s.hasAura(spellID) {
		s.removeAura(spellID)
	}
	s.removeOwnerPetAurasForSpell(ctx, spellID)
	for i := range s.player.Spells {
		if s.player.Spells[i].ID == spellID {
			s.player.Spells = append(s.player.Spells[:i], s.player.Spells[i+1:]...)
			break
		}
	}
	unlearnBuf := protocol.NewBuffer(4)
	unlearnBuf.WriteU32(spellID)
	_ = s.write(uint16(protocol.OpcodeSMSG_REMOVED_SPELL), unlearnBuf.Bytes(), true)
	// Player::RemoveSpell (Player.cpp:3884-3887): unlearning Titan's Grip
	// (46917) while the capability flag is set removes the penalty aura
	// and clears the flag. The flag is derivable from the spell list,
	// which already dropped the spell above; the penalty spell id comes
	// from the DBC row of 46917 itself (the effect's MiscValue), since
	// checkTitanGripPenalty can no longer see the unlearned spell.
	if spellID == 46917 {
		if penaltyID := s.titanGripPenaltySpellID(spellID); penaltyID != 0 {
			s.removeAura(penaltyID)
		}
	}
	s.autoUnequipOffhandIfNeeded(ctx)
}

// removeSpellCascade mirrors Player::RemoveSpell (Player.cpp:3699) as used
// by Player::ActivateSpec: before dropping the spell it recursively unlearns
// (1) known non-talent higher ranks via GetNextSpellInChain
// (Player.cpp:3709-3713) and (2) spells gated on it via
// SpellsRequiringSpellMap (Player.cpp:3715-3718); after the drop it
// recursively unlearns (3) spells the rank teaches through
// SPELL_EFFECT_LEARN_SPELL TriggerSpells (the ActivateSpec loop arm,
// Player.cpp:26363-26366) and (4) spell_learn_spell dependents (the
// SpellLearnSpellMap leg, Player.cpp:3827-3830). Talent points free
// implicitly: Go derives free points from the talent map, which
// loadPlayerTalents rebuilds for the new spec after the swap. The disabled
// mark (RemoveSpell's disabled=true from ActivateSpec), the primary-
// profession free-point arm, and the skill-rank rollback have no Go model
// and stay documented gaps.
func (s *session) removeSpellCascade(ctx context.Context, spellID uint32, visited map[uint32]bool) {
	if s == nil || s.player == nil || spellID == 0 || visited[spellID] || !s.hasLearnedSpell(spellID) {
		return
	}
	visited[spellID] = true
	if s.server != nil {
		if next := s.server.getNextSpellInChain(spellID); next != 0 && s.hasLearnedSpell(next) {
			if cost, err := s.server.Data.TalentSpellCost(next); err == nil && cost == 0 {
				s.removeSpellCascade(ctx, next, visited)
			}
		}
		for _, dep := range s.spellsRequiringSpell(ctx, spellID) {
			s.removeSpellCascade(ctx, dep, visited)
		}
	}
	s.unlearnSpell(ctx, spellID)
	for _, taught := range s.spellsTaughtBySpell(ctx, spellID) {
		s.removeSpellCascade(ctx, taught, visited)
	}
}

// removeSpell mirrors Player::RemoveSpell (Player.cpp:3699) with
// disabled=false, for callers that invoke RemoveSpell directly: the
// .unlearn command (cs_learn.cpp:487, learn_low_rank = !allRanks) and
// EffectUnlearnSpecialization (SpellEffects.cpp:1261, default
// learn_low_rank = true per Player.h:1428). Unlike removeSpellCascade
// (the ActivateSpec disabled=true path), this runs the full body: the
// higher-rank recursion (learn_low_rank=false, non-talent only), the
// spells-requiring-spell recursion, the SpellLearnSkill rollback, the
// spell_learn_spell taught-spell recursion, and the prev-rank
// reactivation. The DBC SPELL_EFFECT_LEARN_SPELL trigger leg is NOT
// part of RemoveSpell (ActivateSpec/ResetTalents run it as a separate
// loop); only the spell_learn_spell table leg recurses here.
func (s *session) removeSpell(ctx context.Context, spellID uint32, learnLowRank bool, visited map[uint32]bool) {
	if s == nil || s.player == nil || spellID == 0 || visited[spellID] || !s.hasLearnedSpell(spellID) {
		return
	}
	visited[spellID] = true
	// Player::RemoveSpell (Player.cpp:3703-3705) early-returns for
	// PLAYERSPELL_TEMPORARY rows; Go hasLearnedSpell does not exclude
	// them, so guard here.
	for i := range s.player.Spells {
		if s.player.Spells[i].ID == spellID && s.player.Spells[i].Temporary {
			return
		}
	}
	if s.server != nil {
		if next := s.server.getNextSpellInChain(spellID); next != 0 && s.hasLearnedSpell(next) {
			if cost, err := s.server.Data.TalentSpellCost(next); err == nil && cost == 0 {
				s.removeSpell(ctx, next, false, visited)
			}
		}
		for _, dep := range s.spellsRequiringSpell(ctx, spellID) {
			s.removeSpell(ctx, dep, true, visited)
		}
	}
	// Re-search: a requiring-spell recursion can already have removed
	// this spell (Player.cpp:3720-3723).
	if !s.hasLearnedSpell(spellID) {
		return
	}
	curActive, curDependent := false, false
	for i := range s.player.Spells {
		if s.player.Spells[i].ID == spellID {
			curActive = s.player.Spells[i].Active
			curDependent = s.player.Spells[i].Dependent
			break
		}
	}
	var talentCost uint32
	if s.server != nil && s.server.Data != nil {
		talentCost, _ = s.server.Data.TalentSpellCost(spellID)
	}
	s.unlearnSpell(ctx, spellID)
	s.applyRemoveSpellSkillLeg(ctx, spellID)
	for _, taught := range s.spellsTaughtByLearnSpellTable(ctx, spellID) {
		s.removeSpell(ctx, taught, true, visited)
	}
	// Prev-rank reactivation (Player.cpp:3845-3881): talent spells take
	// the empty arm; a ranked, non-stackable, active spell re-activates
	// its known prev rank. IsRanked is vacuous here: a prev rank in
	// spell_ranks is rank >= 2 by construction.
	if talentCost == 0 && curActive && s.server != nil {
		if prev := s.server.getPrevSpellInChain(spellID); prev != 0 {
			stackable := false
			if s.server.Data != nil {
				if st, found, err := s.server.Data.SpellStackableWithRanks(spellID); err == nil && found {
					stackable = st
				}
			}
			if !stackable {
				for i := range s.player.Spells {
					if s.player.Spells[i].ID != prev {
						continue
					}
					if s.player.Spells[i].Dependent != curDependent {
						s.player.Spells[i].Dependent = curDependent
					}
					if learnLowRank && !s.player.Spells[i].Active && !s.player.Spells[i].Disabled {
						s.player.Spells[i].Active = true
						if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
							_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_spell SET active = 1 WHERE guid = ? AND spell = ?", s.playerGUID, prev)
						}
						_ = s.write(uint16(protocol.OpcodeSMSG_SUPERCEDED_SPELL), buildSupercededSpell(spellID, prev), true)
						if s.unlearnSpellsPacketNeeded(prev) {
							_ = s.write(uint16(protocol.OpcodeSMSG_SEND_UNLEARN_SPELLS), s.buildUnlearnSpells(ctx, *s.player), true)
						}
					}
					break
				}
			}
		}
	}
}

// spellsTaughtByLearnSpellTable mirrors the SpellLearnSpellMap leg of
// Player::RemoveSpell (Player.cpp:3827-3830): the spell_learn_spell rows
// for spellID. It deliberately excludes the DBC SPELL_EFFECT_LEARN_SPELL
// triggers that spellsTaughtBySpell also returns; those are not part of
// RemoveSpell.
func (s *session) spellsTaughtByLearnSpellTable(ctx context.Context, spellID uint32) []uint32 {
	var out []uint32
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return out
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT SpellID FROM spell_learn_spell WHERE entry = ?", spellID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var taught int64
		if rows.Scan(&taught) == nil && taught > 0 {
			out = append(out, uint32(taught))
		}
	}
	return out
}

// spellLearnSkillNode mirrors SpellLearnSkillNode (SpellMgr.h:518) as
// built by LoadSpellLearnSkills (SpellMgr.cpp:1037): DBC-derived only,
// the first SPELL_EFFECT_SKILL or SPELL_EFFECT_DUAL_WIELD effect wins.
type spellLearnSkillNode struct {
	skill    uint16
	step     uint16
	value    uint16
	maxvalue uint16
}

func (s *session) spellLearnSkill(spellID uint32) (spellLearnSkillNode, bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return spellLearnSkillNode{}, false
	}
	spell, ok, err := s.server.Data.Spell(spellID)
	if err != nil || !ok {
		return spellLearnSkillNode{}, false
	}
	for _, eff := range spell.Effects {
		switch eff.Effect {
		case spellEffectSkill:
			node := spellLearnSkillNode{skill: uint16(eff.MiscValue), step: uint16(eff.CalcValue())}
			if node.skill == 762 {
				node.value = node.step * 75
			} else {
				node.value = 1
			}
			node.maxvalue = node.step * 75
			return node, true
		case spellEffectDualWield:
			return spellLearnSkillNode{skill: 118, step: 1, value: 1, maxvalue: 1}, true
		}
	}
	return spellLearnSkillNode{}, false
}

// applyRemoveSpellSkillLeg mirrors the SpellLearnSkill arm of
// Player::RemoveSpell (Player.cpp:3766-3806): a first-rank unlearn drops
// the skill; otherwise the skill rolls back to the nearest lower rank's
// setting, clamped by the player's current values.
func (s *session) applyRemoveSpellSkillLeg(ctx context.Context, spellID uint32) {
	node, ok := s.spellLearnSkill(spellID)
	if !ok || s.player == nil {
		return
	}
	prev := uint32(0)
	if s.server != nil {
		prev = s.server.getPrevSpellInChain(spellID)
	}
	if prev == 0 {
		s.setSkillFull(ctx, node.skill, 0, 0, 0)
		return
	}
	prevSkill, found := s.spellLearnSkill(prev)
	for !found && prev != 0 {
		prev = s.server.getPrevSpellInChain(prev)
		if prev == 0 {
			break
		}
		prevSkill, found = s.spellLearnSkill(s.server.getFirstSpellInChain(prev))
	}
	if !found {
		s.setSkillFull(ctx, node.skill, 0, 0, 0)
		return
	}
	skillValue := s.getSkillValue(uint32(node.skill))
	if skillValue > uint32(prevSkill.value) {
		skillValue = uint32(prevSkill.value)
	}
	skillMax := s.getSkillMaxValue(uint32(node.skill))
	newMax := uint32(prevSkill.maxvalue)
	if newMax == 0 {
		newMax = uint32(s.player.Level) * 5
	}
	if skillMax > newMax {
		skillMax = newMax
	}
	s.setSkillFull(ctx, node.skill, prevSkill.step, uint16(skillValue), uint16(skillMax))
}

// setSkillFull mirrors Player::SetSkill for the RemoveSpell skill leg:
// all-zero clears the skill row, otherwise the row takes the given
// step/value/max. Go's character_skills writes carry no step column
// (see setOrUpdateSkill), so step lives in memory only. The non-zero arm
// runs LearnSkillRewardedSpells plus the REACH_SKILL_LEVEL and
// LEARN_SKILL_LEVEL criteria, as SetSkill does; the all-zero arm stays
// row-drop-only (the prior tick's deliberate boundary: recursing the
// skill-ability RemoveSpell cascade there would change .unlearn behavior).
func (s *session) setSkillFull(ctx context.Context, skillID, step, value, max uint16) {
	if s.player == nil {
		return
	}
	if step == 0 && value == 0 && max == 0 {
		newSkills := make([]playerSkill, 0, len(s.player.Skills))
		for _, sk := range s.player.Skills {
			if sk.Skill != skillID {
				newSkills = append(newSkills, sk)
			}
		}
		s.player.Skills = newSkills
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_skills WHERE guid = ? AND skill = ?", s.playerGUID, skillID)
		}
		s.sendPlayerUpdate()
		return
	}
	for i := range s.player.Skills {
		if s.player.Skills[i].Skill == skillID {
			s.player.Skills[i].Step = step
			s.player.Skills[i].Value = value
			s.player.Skills[i].Max = max
			if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_skills SET value = ?, max = ? WHERE guid = ? AND skill = ?", value, max, s.playerGUID, skillID)
			}
			s.sendPlayerUpdate()
			s.learnSkillRewardedSpells(ctx, uint32(skillID), uint32(value))
			s.setAchievementCriteria(criteriaTypeReachSkillLevel, uint32(skillID), uint32(value))
			s.setAchievementCriteria(criteriaTypeLearnSkillLevel, uint32(skillID), uint32(max))
			return
		}
	}
	s.player.Skills = append(s.player.Skills, playerSkill{Skill: skillID, Step: step, Value: value, Max: max})
	s.reapplySkillBonusAuras(uint32(skillID))
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, ?, ?)", s.playerGUID, skillID, value, max)
	}
	s.sendPlayerUpdate()
	s.learnSkillRewardedSpells(ctx, uint32(skillID), uint32(value))
	s.setAchievementCriteria(criteriaTypeReachSkillLevel, uint32(skillID), uint32(value))
	s.setAchievementCriteria(criteriaTypeLearnSkillLevel, uint32(skillID), uint32(max))
}

func (s *session) getSkillMaxValue(skillID uint32) uint32 {
	if s.player == nil {
		return 0
	}
	for _, sk := range s.player.Skills {
		if uint32(sk.Skill) == skillID {
			return uint32(sk.Max)
		}
	}
	return 0
}

// unlearnSpellsPacketNeeded mirrors the skill-line-ability half of
// IsUnlearnSpellsPacketNeededForSpell (Player.cpp:3234): the spell has
// skill-line abilities, none of which the client already supercedes.
// The ranked/non-stackable gates already hold at the call site.
func (s *session) unlearnSpellsPacketNeeded(spellID uint32) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	abilities, found, err := s.server.Data.SkillLineAbilities(spellID)
	if err != nil || !found || len(abilities) == 0 {
		return false
	}
	for _, ability := range abilities {
		if ability.SupercededBySpell != 0 {
			return false
		}
	}
	return true
}

// spellsRequiringSpell mirrors the SpellsRequiringSpellMap lookup in
// Player::RemoveSpell (Player.cpp:3715-3718): every spell whose
// spell_required row gates on spellID.
func (s *session) spellsRequiringSpell(ctx context.Context, spellID uint32) []uint32 {
	var out []uint32
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return out
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT spell_id FROM spell_required_spell WHERE req_spell = ?", spellID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var dep int64
		if rows.Scan(&dep) == nil && dep > 0 {
			out = append(out, uint32(dep))
		}
	}
	return out
}

// spellsTaughtBySpell mirrors the two taught-spell legs of
// Player::RemoveSpell: the DBC SPELL_EFFECT_LEARN_SPELL TriggerSpells
// (Player.cpp:26363-26366, the ActivateSpec loop arm) and the
// SpellLearnSpellMap bounds over spell_learn_spell (Player.cpp:3827-3830).
func (s *session) spellsTaughtBySpell(ctx context.Context, spellID uint32) []uint32 {
	var out []uint32
	if s == nil || s.server == nil {
		return out
	}
	if s.server.Data != nil {
		if spell, ok, err := s.server.Data.Spell(spellID); err == nil && ok {
			for _, eff := range spell.Effects {
				if eff.Effect == spellEffectLearnSpell && eff.TriggerSpell > 0 {
					out = append(out, eff.TriggerSpell)
				}
			}
		}
	}
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT SpellID FROM spell_learn_spell WHERE entry = ?", spellID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var taught int64
				if rows.Scan(&taught) == nil && taught > 0 {
					out = append(out, uint32(taught))
				}
			}
		}
	}
	return out
}

// learnTalentDirect grants a talent rank bypassing the talent-point gate,
// mirroring the LearnSpell + AddTalent pair in HandleLearnAllTalentsCommand
// (cs_learn.cpp:280-283). Persistence follows learnTalent (skills.go): the
// old rank's rows are cleared, character_talent is inserted, and the spell
// itself rides the normal learnSpell path.
func (s *session) learnTalentDirect(ctx context.Context, talentID uint32, rank uint8, spellID uint32) {
	if s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	if s.player.Talents == nil {
		s.player.Talents = make(map[uint32]uint8)
	}
	if cur, has := s.player.Talents[talentID]; has && cur == rank {
		return
	}
	var oldSpellID uint32
	if cur, has := s.player.Talents[talentID]; has {
		if tEntry, ok, err := s.server.Data.Talent(talentID); err == nil && ok && uint32(cur) < uint32(len(tEntry.SpellRank)) {
			oldSpellID = tEntry.SpellRank[cur]
		}
	}
	s.player.Talents[talentID] = rank
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		if oldSpellID > 0 {
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_talent WHERE guid = ? AND spell = ? AND talentGroup = ?", s.playerGUID, oldSpellID, s.player.ActiveTalentGroup)
			s.unlearnSpell(ctx, oldSpellID)
		}
		_, _ = cdb.ExecContext(ctx, "INSERT INTO character_talent (guid, spell, talentGroup) VALUES (?, ?, ?)", s.playerGUID, spellID, s.player.ActiveTalentGroup)
	}
	s.learnSpell(ctx, spellID)
}

// learnSkillRecipesHelper mirrors HandleLearnSkillRecipesHelper
// (cs_learn.cpp:434-462): learn every non-superseded, race/class-fitting
// recipe spell of a skill line. IsSpellValid is assumed (documented gap,
// same as the other arms).
func (s *session) learnSkillRecipesHelper(ctx context.Context, target *session, skillID uint32) {
	if s.server == nil || s.server.Data == nil || target.player == nil {
		return
	}
	abilities, ok, err := s.server.Data.SkillLineAbilitiesForSkill(skillID)
	if err != nil || !ok {
		return
	}
	classMask := uint32(1) << (target.player.Class - 1)
	for _, ability := range abilities {
		if ability.SupercededBySpell != 0 {
			continue
		}
		if ability.RaceMask != 0 {
			continue
		}
		if ability.ClassMask != 0 && ability.ClassMask&classMask == 0 {
			continue
		}
		if _, found, err := s.server.Data.Spell(ability.Spell); err != nil || !found {
			continue
		}
		target.learnSpell(ctx, ability.Spell)
	}
}

// learnProfessionSkillLines iterates SkillLine.dbc for primary/secondary
// professions with recipes (cs_learn.cpp:375-378, 407-412:
// SKILL_CATEGORY_PROFESSION = 11 / SKILL_CATEGORY_SECONDARY = 9, CanLink).
func (s *session) learnProfessionSkillLines() []uint32 {
	var ids []uint32
	if s.server == nil || s.server.Data == nil {
		return ids
	}
	file, err := s.server.Data.File("SkillLine")
	if err != nil {
		return ids
	}
	for i := 0; i < file.Records(); i++ {
		rec, err := file.Record(i)
		if err != nil {
			continue
		}
		category, err := rec.Int32(1) // CategoryID (DBCStructure.h:1314)
		if err != nil || (category != 9 && category != 11) {
			continue
		}
		canLink, err := rec.Uint32(55) // CanLink (DBCStructure.h:1323)
		if err != nil || canLink == 0 {
			continue
		}
		if id, err := rec.Uint32(0); err == nil && id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// handleCmdLearn mirrors the learn command table (cs_learn.cpp:64-94):
//
//	learn <spell> [all]            -> HandleLearnCommand            (RBAC_PERM_COMMAND_LEARN, 417)
//	learn all blizzard             -> HandleLearnAllGMCommand       (RBAC_PERM_COMMAND_LEARN_ALL_GM, 424)
//	learn all crafts               -> HandleLearnAllCraftsCommand   (RBAC_PERM_COMMAND_LEARN_ALL_CRAFTS, 425)
//	learn all debug                -> HandleLearnDebugSpellsCommand (RBAC_PERM_COMMAND_LEARN, 417)
//	learn all default              -> HandleLearnAllDefaultCommand  (RBAC_PERM_COMMAND_LEARN_ALL_DEFAULT, 426)
//	learn all languages            -> HandleLearnAllLangCommand     (RBAC_PERM_COMMAND_LEARN_ALL_LANG, 427)
//	learn all recipes <profession> -> HandleLearnAllRecipesCommand  (RBAC_PERM_COMMAND_LEARN_ALL_RECIPES, 428)
//	learn all talents              -> HandleLearnAllTalentsCommand  (RBAC_PERM_COMMAND_LEARN_ALL_TALENTS, 423)
//	learn all pettalents           -> HandleLearnAllPetTalentsCommand (RBAC_PERM_COMMAND_LEARN_MY_PETTALENTS, 421)
//	learn my trainer               -> HandleLearnMySpellsCommand    (RBAC_PERM_COMMAND_LEARN_ALL_MY_SPELLS, 422)
//	learn my quests                -> HandleLearnMyQuestsCommand    (RBAC_PERM_COMMAND_LEARN_ALL_MY_SPELLS, 422)
func (s *session) handleCmdLearn(ctx context.Context, args []string) {
	const syntax = "Syntax: .learn <spellId> [all] | .learn all blizzard|crafts|debug|default|languages|recipes <profession>|talents|pettalents | .learn my trainer|quests"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	deny := func(perm uint32) bool {
		if !s.commandAllowed(ctx, perm) {
			s.sendNotification("You do not have permission to use that command.")
			return true
		}
		return false
	}
	sub := strings.ToLower(args[0])
	switch {
	case strings.HasPrefix("all", sub):
		s.handleLearnAll(ctx, args[1:], deny)
	case strings.HasPrefix("my", sub):
		s.handleLearnMy(ctx, args[1:])
	default:
		// Bare arm: HandleLearnCommand (cs_learn.cpp:99-133). The Trinity
		// "" table entry merges into the parent node, so any non-"all"/"my"
		// first token is the spell argument.
		if deny(permissionCommandLearn) {
			return
		}
		s.handleLearnSpell(ctx, args)
	}
}

// handleLearnSpell ports HandleLearnCommand (cs_learn.cpp:99-133).
func (s *session) handleLearnSpell(ctx context.Context, args []string) {
	spellID, ok := s.learnParseSpellID(args[0])
	if !ok {
		return
	}
	allRanks, ok := s.learnParseAllRanks(args)
	if !ok {
		s.sendSysMessage("Syntax: .learn <spellId> [all]")
		return
	}
	target, ok := s.learnTargetOrSelf()
	if !ok {
		return
	}
	if !allRanks && target.hasLearnedSpell(spellID) {
		// LANG_YOU_KNOWN_SPELL / LANG_TARGET_KNOWN_SPELL inlined from TDB
		// enUS (no in-tree trinity_string seed); GetNameLink has no Go
		// bridge so the plain name is used, per the tree convention.
		if target == s {
			s.sendSysMessage("You already know that spell.")
		} else {
			s.sendSysMessage(fmt.Sprintf("%s already knows that spell.", target.player.Name))
		}
		return
	}
	target.learnSpell(ctx, spellID)
	if allRanks && s.server != nil {
		for next := s.server.getNextSpellInChain(spellID); next != 0; next = s.server.getNextSpellInChain(next) {
			target.learnSpell(ctx, next)
		}
	}
	// Player::AddSpell (Player.cpp:3499-3505): a runtime-learned spell with
	// GetTalentSpellCost(spellId) > 0 and SPELL_EFFECT_LEARN_SPELL is cast
	// triggered on learn — bridged in learnSpell. AddSpell carries no
	// SendTalentsInfoData resend (that arm lives in Player::LearnTalent,
	// Player.cpp:2837, which Go's learnTalentDirect mirrors with
	// sendTalentsInfo(false)); nothing else is missing here.
}

// handleLearnAll ports the learn-all sub-table (cs_learn.cpp:46-57).
func (s *session) handleLearnAll(ctx context.Context, args []string, deny func(uint32) bool) {
	const allSyntax = "Syntax: .learn all blizzard|crafts|debug|default|languages|recipes <profession>|talents|pettalents"
	if len(args) == 0 {
		s.sendSysMessage(allSyntax)
		return
	}
	sub := strings.ToLower(args[0])
	switch {
	case strings.HasPrefix("blizzard", sub):
		// HandleLearnAllGMCommand (cs_learn.cpp:184-199): every spell whose
		// skill lines include SKILL_INTERNAL, taught to the invoker.
		if deny(permissionCommandLearnAllGM) {
			return
		}
		if s.server == nil || s.server.Data == nil {
			s.sendSysMessage("Spell data is unavailable.")
			return
		}
		file, err := s.server.Data.File("Spell")
		if err != nil {
			s.sendSysMessage("Spell data is unavailable.")
			return
		}
		for i := 0; i < file.Records(); i++ {
			rec, err := file.Record(i)
			if err != nil {
				continue
			}
			id, err := rec.Uint32(0)
			if err != nil || id == 0 {
				continue
			}
			if _, found, err := s.server.Data.Spell(id); err != nil || !found {
				continue
			}
			abilities, ok, err := s.server.Data.SkillLineAbilities(id)
			if err != nil || !ok {
				continue
			}
			for _, ability := range abilities {
				if ability.SkillLine == skillInternalID {
					s.learnSpell(ctx, id)
					break
				}
			}
		}
		// LANG_LEARNING_GM_SKILLS inlined from TDB enUS.
		s.sendSysMessage("You have learned all GM skills.")
	case strings.HasPrefix("crafts", sub):
		// HandleLearnAllCraftsCommand (cs_learn.cpp:370-393).
		if deny(permissionCommandLearnAllCrafts) {
			return
		}
		target, ok := s.learnTargetOrSelf()
		if !ok {
			return
		}
		for _, skillID := range s.learnProfessionSkillLines() {
			s.learnSkillRecipesHelper(ctx, target, skillID)
		}
		// LANG_COMMAND_LEARN_ALL_CRAFT inlined from TDB enUS.
		s.sendSysMessage("All crafts have been learned.")
	case strings.HasPrefix("debug", sub):
		// HandleLearnDebugSpellsCommand (cs_learn.cpp:322-331): the seven
		// hardcoded debug spells, taught to the invoker, no message.
		if deny(permissionCommandLearn) {
			return
		}
		for _, id := range learnDebugSpellIDs {
			s.learnSpell(ctx, id)
		}
	case strings.HasPrefix("default", sub):
		// HandleLearnAllDefaultCommand (cs_learn.cpp:333-345): blocked, not
		// stubbed. LearnDefaultSkills needs the playercreateinfo skill/spell
		// sets, LearnCustomSpells the same, and LearnQuestRewardedSpells a
		// quest-template enumeration plus the rewarded-quest set — none have
		// a Go bridge.
		if deny(permissionCommandLearnAllDefault) {
			return
		}
		s.sendSysMessage("learn all default is not supported: default skill/spell/quest-reward learning has no enumeration bridge.")
	case strings.HasPrefix("languages", sub):
		// HandleLearnAllLangCommand (cs_learn.cpp:313-320): the
		// lang_description spell ids, taught to the invoker.
		if deny(permissionCommandLearnAllLang) {
			return
		}
		for _, id := range learnLanguageSpellIDs {
			s.learnSpell(ctx, id)
		}
		// LANG_COMMAND_LEARN_ALL_LANG inlined from TDB enUS.
		s.sendSysMessage("All languages have been learned.")
	case strings.HasPrefix("recipes", sub):
		// HandleLearnAllRecipesCommand (cs_learn.cpp:395-432).
		if deny(permissionCommandLearnAllRecipes) {
			return
		}
		s.handleLearnAllRecipes(ctx, args[1:])
	case strings.HasPrefix("talents", sub):
		// HandleLearnAllTalentsCommand (cs_learn.cpp:256-294).
		if deny(permissionCommandLearnAllTalents) {
			return
		}
		s.handleLearnAllTalents(ctx)
	case strings.HasPrefix("pettalents", sub):
		// HandleLearnAllPetTalentsCommand (cs_learn.cpp:296-311): blocked,
		// not stubbed. Pet::learnSpellHighRank, the pet talent DB rows, and
		// CreatureFamily PetTalentType gating have no Go bridge.
		if deny(permissionCommandLearnMyPetTalents) {
			return
		}
		s.sendSysMessage("learn all pettalents is not supported: pet talent learning has no bridge.")
	default:
		s.sendSysMessage(allSyntax)
	}
}

// handleLearnAllRecipes ports HandleLearnAllRecipesCommand
// (cs_learn.cpp:395-432): match a profession by (case-insensitive) name
// substring, learn its recipes, and raise the skill to max.
func (s *session) handleLearnAllRecipes(ctx context.Context, args []string) {
	target, ok := s.learnSelectedPlayer("Player not found.")
	if !ok {
		return
	}
	if len(args) == 0 || strings.TrimSpace(strings.Join(args, " ")) == "" {
		return
	}
	namePart := strings.ToLower(strings.Join(args, " "))
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Skill data is unavailable.")
		return
	}
	file, err := s.server.Data.File("SkillLine")
	if err != nil {
		s.sendSysMessage("Skill data is unavailable.")
		return
	}
	var skillID uint32
	var skillName string
	for i := 0; i < file.Records(); i++ {
		rec, err := file.Record(i)
		if err != nil {
			continue
		}
		category, err := rec.Int32(1)
		if err != nil || (category != 9 && category != 11) {
			continue
		}
		canLink, err := rec.Uint32(55)
		if err != nil || canLink == 0 {
			continue
		}
		// The C++ checks every locale's DisplayName except the session DBC
		// locale; Go only carries enUS (field 3) - documented gap.
		name, err := rec.String(3)
		if err != nil || name == "" {
			continue
		}
		if strings.Contains(strings.ToLower(name), namePart) {
			if id, err := rec.Uint32(0); err == nil && id != 0 {
				skillID, skillName = id, name
				break
			}
		}
	}
	if skillID == 0 {
		return
	}
	s.learnSkillRecipesHelper(ctx, target, skillID)
	// C++: maxLevel = target->GetPureMaxSkillValue(skill) (the target's
	// current max, 0 when the skill is unknown), then
	// SetSkill(id, GetSkillStep(id), maxLevel, maxLevel).
	var maxLevel uint16
	for _, sk := range target.player.Skills {
		if sk.Skill == uint16(skillID) {
			maxLevel = sk.Max
			break
		}
	}
	step, _, _ := s.server.Data.SkillStep(skillID, target.player.Race, target.player.Class, maxLevel)
	found := false
	for i := range target.player.Skills {
		if target.player.Skills[i].Skill == uint16(skillID) {
			target.player.Skills[i].Step = step
			target.player.Skills[i].Value = maxLevel
			target.player.Skills[i].Max = maxLevel
			found = true
			break
		}
	}
	if !found {
		target.player.Skills = append(target.player.Skills, playerSkill{Skill: uint16(skillID), Step: step, Value: maxLevel, Max: maxLevel})
		target.reapplySkillBonusAuras(skillID)
	}
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, ?, ?)", target.playerGUID, skillID, maxLevel, maxLevel)
	}
	// LANG_COMMAND_LEARN_ALL_RECIPES inlined from TDB enUS.
	s.sendSysMessage(fmt.Sprintf("You have learned all %s recipes.", skillName))
}

// handleLearnAllTalents ports HandleLearnAllTalentsCommand
// (cs_learn.cpp:256-294): learn the highest rank of every talent whose tab
// matches the invoker's class. SetFreeTalentPoints(0) has no Go equivalent -
// the Go tree derives free points from level minus spent ranks, which lands
// at 0 once every talent is spent (documented gap).
func (s *session) handleLearnAllTalents(ctx context.Context) {
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Talent data is unavailable.")
		return
	}
	talents, err := s.server.Data.File("Talent")
	if err != nil {
		s.sendSysMessage("Talent data is unavailable.")
		return
	}
	tabs, err := s.server.Data.File("TalentTab")
	if err != nil {
		s.sendSysMessage("Talent data is unavailable.")
		return
	}
	classMask := uint32(1) << (s.player.Class - 1)
	for i := 0; i < talents.Records(); i++ {
		rec, err := talents.Record(i)
		if err != nil {
			continue
		}
		tabID, err := rec.Uint32(1) // TabID (DBCStructure.h:1684)
		if err != nil {
			continue
		}
		tab, found := tabs.Find(tabID)
		if !found {
			continue
		}
		tabClassMask, err := tab.Uint32(20) // ClassMask (DBCStructure.h:1687)
		if err != nil || tabClassMask&classMask == 0 {
			continue
		}
		talentID, err := rec.Uint32(0)
		if err != nil || talentID == 0 {
			continue
		}
		// Highest talent rank, like the C++ MAX_TALENT_RANK-1 downward scan.
		var spellID uint32
		var rank uint8
		for r := 4; r >= 0; r-- {
			if sp, err := rec.Uint32(4 + r); err == nil && sp != 0 {
				spellID, rank = sp, uint8(r)
				break
			}
		}
		if spellID == 0 {
			continue
		}
		if _, found, err := s.server.Data.Spell(spellID); err != nil || !found {
			continue
		}
		s.learnTalentDirect(ctx, talentID, rank, spellID)
	}
	_ = s.sendTalentsInfo(false)
	// LANG_COMMAND_LEARN_CLASS_TALENTS inlined from TDB enUS.
	s.sendSysMessage("You have learned all your class talents.")
}

// handleLearnMy ports the learn-my sub-table (cs_learn.cpp:60-64). Both arms
// are blocked, not stubbed.
func (s *session) handleLearnMy(ctx context.Context, args []string) {
	const mySyntax = "Syntax: .learn my trainer|quests"
	if len(args) == 0 {
		s.sendSysMessage(mySyntax)
		return
	}
	if !s.commandAllowed(ctx, permissionCommandLearnAllMySpells) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	switch sub := strings.ToLower(args[0]); {
	case strings.HasPrefix("trainer", sub):
		// HandleLearnMySpellsCommand (cs_learn.cpp:219-254): iterates every
		// class trainer's spell list. The Go tree loads trainer spells per
		// trainer GUID only; class-trainer enumeration has no bridge.
		s.sendSysMessage("learn my trainer is not supported: class trainer enumeration has no bridge.")
	case strings.HasPrefix("quests", sub):
		// HandleLearnMyQuestsCommand (cs_learn.cpp:201-210): iterates every
		// quest template. The Go tree has no quest-template enumeration
		// bridge.
		s.sendSysMessage("learn my quests is not supported: quest template enumeration has no bridge.")
	default:
		s.sendSysMessage(mySyntax)
	}
}

// handleCmdUnLearn ports HandleUnLearnCommand (cs_learn.cpp:464-488).
// Syntax: .unlearn <spellId> [all] (RBAC_PERM_COMMAND_UNLEARN, 429).
func (s *session) handleCmdUnLearn(ctx context.Context, args []string) {
	const syntax = "Syntax: .unlearn <spellId> [all]"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if !s.commandAllowed(ctx, permissionCommandUnlearn) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	spellID, ok := s.learnParseSpellID(args[0])
	if !ok {
		return
	}
	allRanks, ok := s.learnParseAllRanks(args)
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	// The C++ requires a selected player (LANG_NO_CHAR_SELECTED).
	target, ok := s.learnSelectedPlayer("No character selected.")
	if !ok {
		return
	}
	unlearnID := spellID
	if allRanks && s.server != nil {
		if first := s.server.getFirstSpellInChain(spellID); first != 0 {
			unlearnID = first
		}
	}
	if target.hasLearnedSpell(unlearnID) {
		target.removeSpell(ctx, unlearnID, !allRanks, make(map[uint32]bool))
	} else {
		// LANG_FORGET_SPELL inlined from TDB enUS.
		s.sendSysMessage("You have forgotten that spell.")
	}
	// HandleUnLearnCommand (cs_learn.cpp:487-492) resends the talent
	// panel whenever the unlearned spell carries a talent cost, whether
	// or not it was known. The talent-point refund arm
	// (m_usedTalentCount -= GetTalentSpellCost, Player.cpp:3751-3757)
	// has no Go counterpart: free points derive from the talent map,
	// which RemoveSpell does not touch, so a .unlearn of a talent spell
	// leaves Go's free-point count unchanged where C++ refunds it.
	if s.server != nil && s.server.Data != nil {
		if cost, err := s.server.Data.TalentSpellCost(unlearnID); err == nil && cost > 0 {
			_ = target.sendTalentsInfo(false)
		}
	}
}
