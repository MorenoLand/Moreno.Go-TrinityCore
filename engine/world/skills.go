package world

import (
	"context"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	maxTalentsCount   uint32 = 150
	maxTalentRank     uint32 = 5
	maxGlyphSlotIndex uint8  = 6
)

// freeTalentPoints mirrors Player::GetFreeTalentPoints (Player.cpp:25462).
// Level < 10 grants 0 talent points.
// Level >= 10 grants (level - 9) talent points minus all spent points.
func (s *session) freeTalentPoints() uint32 {
	if s.player == nil || s.player.Level < 10 {
		return 0
	}
	totalPoints := uint32(s.player.Level - 9)
	var spent uint32
	for _, rank := range s.player.Talents {
		spent += uint32(rank + 1)
	}
	if spent >= totalPoints {
		return 0
	}
	return totalPoints - spent
}

// learnTalent mirrors Player::LearnTalent (Player.cpp:25460).
func (s *session) learnTalent(ctx context.Context, talentID, requestedRank uint32) bool {
	if s.player == nil || requestedRank >= maxTalentRank {
		return false
	}
	if s.player.Talents == nil {
		s.player.Talents = make(map[uint32]uint8)
	}
	curRank, has := s.player.Talents[talentID]
	if has && curRank >= uint8(requestedRank) {
		return false
	}
	neededPoints := requestedRank + 1
	if has {
		neededPoints = requestedRank - uint32(curRank)
	}
	if s.freeTalentPoints() < neededPoints {
		return false
	}

	var oldSpellID uint32
	if has && s.server != nil && s.server.Data != nil {
		if tEntry, ok, err := s.server.Data.Talent(talentID); err == nil && ok && uint32(curRank) < uint32(len(tEntry.SpellRank)) {
			oldSpellID = tEntry.SpellRank[curRank]
		}
	}

	var spellID uint32
	if s.server != nil && s.server.Data != nil {
		if tEntry, ok, err := s.server.Data.Talent(talentID); err == nil && ok && requestedRank < uint32(len(tEntry.SpellRank)) {
			spellID = tEntry.SpellRank[requestedRank]
		}
	}
	if spellID == 0 {
		spellID = talentID*10 + requestedRank + 1
	}

	s.player.Talents[talentID] = uint8(requestedRank)

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		if oldSpellID > 0 {
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_talent WHERE guid = ? AND spell = ? AND talentGroup = ?", s.playerGUID, oldSpellID, s.player.ActiveTalentGroup)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_spell WHERE guid = ? AND spell = ?", s.playerGUID, oldSpellID)
		}
		_, _ = cdb.ExecContext(ctx, "INSERT INTO character_talent (guid, spell, talentGroup) VALUES (?, ?, ?)", s.playerGUID, spellID, s.player.ActiveTalentGroup)
		_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", s.playerGUID, spellID)
		s.updateAchievementCriteria(criteriaTypeLearnSpell, spellID, 1)
	}
	return true
}

func (s *session) loadTalentsForGroup(group uint8) map[uint32]uint8 {
	talents := make(map[uint32]uint8)
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return talents
	}
	cdb := s.server.CharactersStore.DB
	rows, err := cdb.Query("SELECT spell FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, group)
	if err != nil {
		return talents
	}
	defer rows.Close()
	for rows.Next() {
		var spellID int64
		if err := rows.Scan(&spellID); err == nil && spellID > 0 {
			var tid uint32
			var r uint8
			var found bool
			if s.server.Data != nil {
				tid, r, found = s.server.Data.TalentBySpell(uint32(spellID))
			}
			if !found && spellID > 10 {
				tid = uint32((spellID - 1) / 10)
				r = uint8((spellID - 1) % 10)
				found = true
			}
			if found {
				talents[tid] = r
			}
		}
	}
	return talents
}

// sendTalentsInfo mirrors Player::SendTalentsInfoData (Player.cpp:25909).
func (s *session) sendTalentsInfo(pet bool) error {
	buf := protocol.NewBuffer(64)
	if pet {
		unspentPoints := uint32(0)
		var learnedTalents []struct {
			talentID uint32
			rank     uint8
		}
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			var petID, petType, level int64
			err := s.server.CharactersStore.DB.QueryRow("SELECT id, PetType, level FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&petID, &petType, &level)
			if err == nil && petType == 1 && level >= 20 { // Hunter pet
				maxPoints := uint32((level - 16) / 4)
				rows, qErr := s.server.CharactersStore.DB.Query("SELECT spell FROM pet_spell WHERE guid = ?", petID)
				if qErr == nil {
					defer rows.Close()
					usedPoints := uint32(0)
					for rows.Next() {
						var spID int64
						if rows.Scan(&spID) == nil && s.server.Data != nil {
							if tID, rank, ok := s.server.Data.TalentBySpell(uint32(spID)); ok {
								learnedTalents = append(learnedTalents, struct {
									talentID uint32
									rank     uint8
								}{tID, rank})
								usedPoints += uint32(rank + 1)
							}
						}
					}
					if maxPoints > usedPoints {
						unspentPoints = maxPoints - usedPoints
					}
				}
			}
		}
		buf.WriteU8(1)
		buf.WriteU32(unspentPoints)
		buf.WriteU8(uint8(len(learnedTalents)))
		for _, t := range learnedTalents {
			buf.WriteU32(t.talentID)
			buf.WriteU8(t.rank)
		}
		return s.write(uint16(protocol.OpcodeSMSG_TALENTS_INFO), buf.Bytes(), true)
	}

	buf.WriteU8(0)
	buf.WriteU32(s.freeTalentPoints())
	specsCount := s.player.TalentGroupsCount
	if specsCount == 0 {
		specsCount = 1
	}
	buf.WriteU8(specsCount)
	buf.WriteU8(s.player.ActiveTalentGroup)

	for spec := uint8(0); spec < specsCount; spec++ {
		talents := s.player.Talents
		if spec != s.player.ActiveTalentGroup {
			talents = s.loadTalentsForGroup(spec)
		}
		talentIDs := make([]uint32, 0, len(talents))
		for tid := range talents {
			talentIDs = append(talentIDs, tid)
		}
		sort.Slice(talentIDs, func(i, j int) bool { return talentIDs[i] < talentIDs[j] })
		buf.WriteU8(uint8(len(talentIDs)))
		for _, tid := range talentIDs {
			rank := talents[tid]
			buf.WriteU32(tid)
			buf.WriteU8(rank)
		}
		buf.WriteU8(maxGlyphSlotIndex)
		for i := uint8(0); i < maxGlyphSlotIndex; i++ {
			glyphID := uint16(0)
			if int(spec) < len(s.player.Glyphs) && int(i) < len(s.player.Glyphs[spec]) {
				glyphID = s.player.Glyphs[spec][i]
			}
			buf.WriteU16(glyphID)
		}
	}
	return s.write(uint16(protocol.OpcodeSMSG_TALENTS_INFO), buf.Bytes(), true)
}

func (s *session) sendResyncRunes() error {
	if s.player == nil || s.player.Class != 6 {
		return nil
	}
	buf := protocol.NewBuffer(16)
	buf.WriteU32(6)
	for _, runeType := range []uint8{0, 0, 1, 1, 2, 2} {
		buf.WriteU8(runeType)
		buf.WriteU8(0)
	}
	return s.write(uint16(protocol.OpcodeSMSG_RESYNC_RUNES), buf.Bytes(), true)
}

// handleLearnTalent processes CMSG_LEARN_TALENT (0x251).
// Reference: WorldSession::HandleLearnTalentOpcode (SkillHandler.cpp:27).
func (s *session) handleLearnTalent(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	talentID, err := r.ReadU32()
	if err != nil {
		return false
	}
	requestedRank, err := r.ReadU32()
	if err != nil {
		return false
	}

	s.learnTalent(ctx, talentID, requestedRank)
	_ = s.sendTalentsInfo(false)
	return true
}

// handleLearnPreviewTalents processes CMSG_LEARN_PREVIEW_TALENTS (0x4C1).
// Reference: WorldSession::HandleLearnPreviewTalents (SkillHandler.cpp:36).
func (s *session) handleLearnPreviewTalents(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	talentsCount, err := r.ReadU32()
	if err != nil {
		return false
	}

	for i := uint32(0); i < talentsCount && i < maxTalentsCount; i++ {
		talentID, err := r.ReadU32()
		if err != nil {
			return false
		}
		talentRank, err := r.ReadU32()
		if err != nil {
			return false
		}
		s.learnTalent(ctx, talentID, talentRank)
	}

	_ = s.sendTalentsInfo(false)
	return true
}

// handleUnlearnSkill processes CMSG_UNLEARN_SKILL (0x202).
// Reference: WorldSession::HandleUnlearnSkillOpcode (SkillHandler.cpp:93).
func (s *session) handleUnlearnSkill(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	skillID, err := r.ReadU32()
	if err != nil {
		return false
	}

	newSkills := make([]playerSkill, 0, len(s.player.Skills))
	for _, sk := range s.player.Skills {
		if uint32(sk.Skill) != skillID {
			newSkills = append(newSkills, sk)
		}
	}
	s.player.Skills = newSkills

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_skills WHERE guid = ? AND skill = ?", s.playerGUID, skillID)
	}

	s.sendPlayerUpdate()
	return true
}

const (
	goldUnit uint32 = 10000
	monthSec        = 30 * 24 * 3600
)

// resetTalentsCost mirrors Player::ResetTalentsCost (Player.cpp:3957): the
// first reset costs 1 gold, then 5, then 10, afterwards increments of 5 gold
// up to a 50 gold cap, decaying 5 gold per month down to a 10 gold floor.
func (s *session) resetTalentsCost() uint32 {
	if s.player == nil {
		return goldUnit
	}
	if s.player.ResetTalentsCost < goldUnit {
		return goldUnit
	}
	if s.player.ResetTalentsCost < 5*goldUnit {
		return 5 * goldUnit
	}
	if s.player.ResetTalentsCost < 10*goldUnit {
		return 10 * goldUnit
	}
	months := (uint32(time.Now().Unix()) - s.player.ResetTalentsTime) / monthSec
	if months > 0 {
		decay := int64(5*goldUnit) * int64(months)
		newCost := int64(s.player.ResetTalentsCost) - decay
		floor := int64(10 * goldUnit)
		if newCost < floor {
			return 10 * goldUnit
		}
		return uint32(newCost)
	}
	newCost := s.player.ResetTalentsCost + 5*goldUnit
	if newCost > 50*goldUnit {
		newCost = 50 * goldUnit
	}
	return newCost
}

// resetTalents mirrors Player::ResetTalents (Player.cpp:3990): remove every
// learned talent spell for the active talent group, refund the full point
// pool, charge the escalating cost (unless free), persist the new cost state,
// and refresh the talent panel. Returns false when the player cannot afford
// the reset.
func (s *session) resetTalents(ctx context.Context, free bool) bool {
	if s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	if len(s.player.Talents) == 0 {
		return true // nothing spent, nothing to reset
	}
	cost := uint32(0)
	if !free {
		cost = s.resetTalentsCost()
		if s.player.Money < cost {
			return false
		}
	}

	// Unlearn the highest known rank spell of every talent.
	for talentID, rank := range s.player.Talents {
		if s.server.Data == nil {
			continue
		}
		if tEntry, ok, err := s.server.Data.Talent(talentID); err == nil && ok && uint32(rank) < uint32(len(tEntry.SpellRank)) {
			spellID := tEntry.SpellRank[rank]
			if spellID != 0 {
				removed := protocol.NewBuffer(4)
				removed.WriteU32(spellID)
				_ = s.write(uint16(protocol.OpcodeSMSG_REMOVED_SPELL), removed.Bytes(), true)
			}
		}
	}

	s.player.Talents = make(map[uint32]uint8)
	cdb := s.server.CharactersStore.DB
	if _, err := cdb.ExecContext(ctx, "DELETE FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, s.player.ActiveTalentGroup); err != nil {
		s.debug("talent reset persistence failed", "account", s.accountName, "error", err)
	}
	if cost > 0 {
		s.player.Money -= cost
		s.player.ResetTalentsCost = cost
		s.player.ResetTalentsTime = uint32(time.Now().Unix())
		s.updateAchievementCriteria(criteriaTypeGoldSpentForTalents, 0, cost)
	}
	s.updateAchievementCriteria(criteriaTypeTalentResets, 0, 1)
	if _, err := cdb.ExecContext(ctx, "UPDATE characters SET resettalents_cost = ?, resettalents_time = ?, money = ? WHERE guid = ?", s.player.ResetTalentsCost, s.player.ResetTalentsTime, s.player.Money, s.playerGUID); err != nil {
		s.debug("talent reset cost persistence failed", "account", s.accountName, "error", err)
	}
	s.sendPlayerUpdate()
	_ = s.sendTalentsInfo(false)
	s.debug("talents reset", "account", s.accountName, "cost", cost, "free", free)
	return true
}
