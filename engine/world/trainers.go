package world

import (
	"context"
	"database/sql"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type trainerSpellRecord struct {
	SpellID       uint32
	ServiceState  uint8
	Cost          uint32
	TalentCost    uint32
	PrimaryProf   uint32
	ReqLevel      uint8
	ReqSkill      uint32
	ReqSkillValue uint32
	ReqSpell      uint32
	ReqSpellChain uint32
	ReqSpell3     uint32
}

const unitNPCFlagTrainer uint32 = 0x00000070

func (s *session) handleTrainerList(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	reader := protocol.NewReader(payload)
	trainerGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	return s.sendTrainerList(ctx, trainerGUID)
}

func (s *session) sendTrainerList(ctx context.Context, trainerGUID uint64) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	if !s.canInteractWithNPC(ctx, trainerGUID, uint64(unitNPCFlagTrainer)) {
		return true
	}
	var creatureEntry uint32
	if creature := s.luaCreature(ctx, trainerGUID); creature != nil {
		creatureEntry = objectUint32OrZero(creature, "Entry")
	}
	if creatureEntry == 0 {
		creatureEntry = uint32((trainerGUID >> 24) & 0x00FFFFFF)
	}
	if creatureEntry == 0 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		spawnGUID := uint32(trainerGUID & 0x00FFFFFF)
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", spawnGUID).Scan(&creatureEntry)
	}

	var spells []trainerSpellRecord
	greeting := "Hello! Ready for some training?"
	var trainerType uint32 = 0

	// 1. Resolve trainer & validate for player
	var trainerID, tType, tReq int64
	var greet sql.NullString
	foundTrainer := false
	err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT t.Id, COALESCE(t.Type, 0), COALESCE(t.Requirement, 0), COALESCE(t.Greeting, 'Hello! Ready for some training?')
		FROM trainer AS t
		WHERE t.Id IN (SELECT TrainerId FROM creature_default_trainer WHERE CreatureId = ?)
		   OR t.Id = ?
		LIMIT 1`, creatureEntry, creatureEntry).Scan(&trainerID, &tType, &tReq, &greet)
	if err == nil {
		foundTrainer = true
		if greet.Valid && greet.String != "" {
			greeting = greet.String
		}
		trainerType = uint32(tType)

		// Check if trainer is valid for player (TrinityCore: if (!IsTrainerValidForPlayer(player)) return;)
		if !s.isTrainerValidForPlayer(uint32(tType), uint32(tReq)) {
			s.debug("trainer not valid for player", "account", s.accountName, "trainer", trainerGUID, "type", tType, "req", tReq)
			return true
		}
		type rawSpell struct {
			spellID       uint32
			moneyCost     uint32
			reqSkill      uint32
			reqSkillValue uint32
			reqLevel      uint8
			reqAb1        uint32
			reqAb2        uint32
			reqAb3        uint32
		}
		var rawList []rawSpell

		rows, qErr := s.server.WorldStore.DB.QueryContext(ctx, `SELECT ts.SpellId, COALESCE(ts.MoneyCost, 0), COALESCE(ts.ReqSkillLine, 0), COALESCE(ts.ReqSkillRank, 0), COALESCE(ts.ReqLevel, 0),
			COALESCE(ts.ReqAbility1, 0), COALESCE(ts.ReqAbility2, 0), COALESCE(ts.ReqAbility3, 0)
			FROM trainer_spell AS ts
			WHERE ts.TrainerId = ?
			ORDER BY ts.ReqLevel, ts.SpellId LIMIT 512`, trainerID)
		if qErr == nil {
			for rows.Next() {
				var spellID, moneyCost, reqSkill, reqSkillValue, reqLevel, reqAb1, reqAb2, reqAb3 int64
				if err := rows.Scan(&spellID, &moneyCost, &reqSkill, &reqSkillValue, &reqLevel, &reqAb1, &reqAb2, &reqAb3); err == nil {
					rawList = append(rawList, rawSpell{
						spellID:       uint32(spellID),
						moneyCost:     uint32(moneyCost),
						reqSkill:      uint32(reqSkill),
						reqSkillValue: uint32(reqSkillValue),
						reqLevel:      uint8(reqLevel),
						reqAb1:        uint32(reqAb1),
						reqAb2:        uint32(reqAb2),
						reqAb3:        uint32(reqAb3),
					})
				}
			}
			_ = rows.Close()
		}

		for _, raw := range rawList {
			if !s.isSpellFitByClassAndRace(raw.spellID) {
				continue
			}
			reqAbs := []uint32{raw.reqAb1, raw.reqAb2, raw.reqAb3}
			serviceState := s.getTrainerSpellState(raw.spellID, raw.reqLevel, raw.reqSkill, raw.reqSkillValue, reqAbs)
			spells = append(spells, trainerSpellRecord{
				SpellID:       raw.spellID,
				ServiceState:  serviceState,
				Cost:          raw.moneyCost,
				ReqLevel:      raw.reqLevel,
				ReqSkill:      raw.reqSkill,
				ReqSkillValue: raw.reqSkillValue,
				ReqSpell:      raw.reqAb1,
				ReqSpellChain: raw.reqAb2,
				ReqSpell3:     raw.reqAb3,
			})
		}
	}

	// 2. Fallback to legacy npc_trainer if no trainer row found
	if !foundTrainer {
		type rawFb struct {
			spellID       uint32
			moneyCost     uint32
			reqSkill      uint32
			reqSkillValue uint32
			reqLevel      uint8
		}
		var rawFbList []rawFb

		fbRows, fbErr := s.server.WorldStore.DB.QueryContext(ctx, `SELECT SpellID, COALESCE(MoneyCost, 0), COALESCE(ReqSkill, 0), COALESCE(ReqSkillValue, 0), COALESCE(ReqLevel, 0)
			FROM npc_trainer WHERE ID = ?
			ORDER BY ReqLevel, SpellID LIMIT 128`, creatureEntry)
		if fbErr == nil {
			for fbRows.Next() {
				var spellID, moneyCost, reqSkill, reqSkillValue, reqLevel int64
				if err := fbRows.Scan(&spellID, &moneyCost, &reqSkill, &reqSkillValue, &reqLevel); err == nil {
					rawFbList = append(rawFbList, rawFb{
						spellID:       uint32(spellID),
						moneyCost:     uint32(moneyCost),
						reqSkill:      uint32(reqSkill),
						reqSkillValue: uint32(reqSkillValue),
						reqLevel:      uint8(reqLevel),
					})
				}
			}
			_ = fbRows.Close()
		}

		for _, raw := range rawFbList {
			if !s.isSpellFitByClassAndRace(raw.spellID) {
				continue
			}
			serviceState := s.getTrainerSpellState(raw.spellID, raw.reqLevel, raw.reqSkill, raw.reqSkillValue, nil)
			spells = append(spells, trainerSpellRecord{
				SpellID:       raw.spellID,
				ServiceState:  serviceState,
				Cost:          raw.moneyCost,
				ReqLevel:      raw.reqLevel,
				ReqSkill:      raw.reqSkill,
				ReqSkillValue: raw.reqSkillValue,
			})
		}
	}

	// 3. Fallback to class trainer ONLY if no trainer row was found at all for this creature (for tests/custom spawns)
	if !foundTrainer && len(spells) == 0 && s.player != nil && s.player.Class > 0 {
		type rawCt struct {
			spellID       uint32
			moneyCost     uint32
			reqSkill      uint32
			reqSkillValue uint32
			reqLevel      uint8
			reqAb1        uint32
			reqAb2        uint32
			reqAb3        uint32
			tType         uint32
			greet         string
		}
		var rawCtList []rawCt

		ctRows, ctErr := s.server.WorldStore.DB.QueryContext(ctx, `SELECT ts.SpellId, COALESCE(ts.MoneyCost, 0), COALESCE(ts.ReqSkillLine, 0), COALESCE(ts.ReqSkillRank, 0), COALESCE(ts.ReqLevel, 0),
			COALESCE(ts.ReqAbility1, 0), COALESCE(ts.ReqAbility2, 0), COALESCE(ts.ReqAbility3, 0),
			COALESCE(t.Type, 0), COALESCE(t.Greeting, 'Hello! Ready for some training?')
			FROM trainer_spell AS ts
			JOIN trainer AS t ON t.Id = ts.TrainerId
			WHERE t.Type = 0 AND t.Requirement = ?
			ORDER BY ts.ReqLevel, ts.SpellId LIMIT 128`, s.player.Class)
		if ctErr == nil {
			for ctRows.Next() {
				var spellID, moneyCost, reqSkill, reqSkillValue, reqLevel, reqAb1, reqAb2, reqAb3, tType int64
				var greet sql.NullString
				if err := ctRows.Scan(&spellID, &moneyCost, &reqSkill, &reqSkillValue, &reqLevel, &reqAb1, &reqAb2, &reqAb3, &tType, &greet); err == nil {
					gStr := ""
					if greet.Valid {
						gStr = greet.String
					}
					rawCtList = append(rawCtList, rawCt{
						spellID:       uint32(spellID),
						moneyCost:     uint32(moneyCost),
						reqSkill:      uint32(reqSkill),
						reqSkillValue: uint32(reqSkillValue),
						reqLevel:      uint8(reqLevel),
						reqAb1:        uint32(reqAb1),
						reqAb2:        uint32(reqAb2),
						reqAb3:        uint32(reqAb3),
						tType:         uint32(tType),
						greet:         gStr,
					})
				}
			}
			_ = ctRows.Close()
		}

		for _, raw := range rawCtList {
			if !s.isSpellFitByClassAndRace(raw.spellID) {
				continue
			}
			if raw.greet != "" {
				greeting = raw.greet
			}
			trainerType = raw.tType
			reqAbs := []uint32{raw.reqAb1, raw.reqAb2, raw.reqAb3}
			serviceState := s.getTrainerSpellState(raw.spellID, raw.reqLevel, raw.reqSkill, raw.reqSkillValue, reqAbs)
			spells = append(spells, trainerSpellRecord{
				SpellID:       raw.spellID,
				ServiceState:  serviceState,
				Cost:          raw.moneyCost,
				ReqLevel:      raw.reqLevel,
				ReqSkill:      raw.reqSkill,
				ReqSkillValue: raw.reqSkillValue,
				ReqSpell:      raw.reqAb1,
				ReqSpellChain: raw.reqAb2,
				ReqSpell3:     raw.reqAb3,
			})
		}
	}

	packet := protocol.NewBuffer(8 + 4 + 4 + len(spells)*38 + len(greeting) + 1)
	packet.WriteU64(trainerGUID)
	packet.WriteU32(trainerType)
	packet.WriteU32(uint32(len(spells)))
	for _, sp := range spells {
		packet.WriteU32(sp.SpellID)
		packet.WriteU8(sp.ServiceState)
		packet.WriteU32(sp.Cost)
		packet.WriteU32(sp.TalentCost)
		packet.WriteU32(sp.PrimaryProf)
		packet.WriteU8(sp.ReqLevel)
		packet.WriteU32(sp.ReqSkill)
		packet.WriteU32(sp.ReqSkillValue)
		packet.WriteU32(sp.ReqSpell)
		packet.WriteU32(sp.ReqSpellChain)
		packet.WriteU32(sp.ReqSpell3)
	}
	packet.WriteCString(greeting)
	_ = s.write(uint16(protocol.OpcodeSMSG_TRAINER_LIST), packet.Bytes(), true)
	s.debug("trainer list sent", "account", s.accountName, "trainer", trainerGUID, "spells", len(spells))
	return true
}

func (s *session) handleTrainerBuySpell(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	reader := protocol.NewReader(payload)
	trainerGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	spellID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if s.hasLearnedSpell(spellID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_TRAINER_BUY_FAILED), buildTrainerBuyFailed(trainerGUID, spellID, 0), true) // AlreadyKnown = 0
		return true
	}
	wdb := s.server.WorldStore.DB
	cdb := s.server.CharactersStore.DB
	if wdb == nil || cdb == nil {
		return true
	}
	if !s.canInteractWithNPC(ctx, trainerGUID, uint64(unitNPCFlagTrainer)) {
		return true
	}
	var creatureEntry uint32
	if creature := s.luaCreature(ctx, trainerGUID); creature != nil {
		creatureEntry = objectUint32OrZero(creature, "Entry")
	}
	if creatureEntry == 0 {
		creatureEntry = uint32((trainerGUID >> 24) & 0x00FFFFFF)
	}
	if creatureEntry == 0 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		spawnGUID := uint32(trainerGUID & 0x00FFFFFF)
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", spawnGUID).Scan(&creatureEntry)
	}

	// 1. Resolve trainer & validate for player
	var trainerID, tType, tReq int64
	var foundTrainer bool
	err = wdb.QueryRowContext(ctx, `SELECT t.Id, COALESCE(t.Type, 0), COALESCE(t.Requirement, 0)
		FROM trainer AS t
		WHERE t.Id IN (SELECT TrainerId FROM creature_default_trainer WHERE CreatureId = ?)
		   OR t.Id = ? LIMIT 1`, creatureEntry, creatureEntry).Scan(&trainerID, &tType, &tReq)
	if err == nil {
		foundTrainer = true
		if !s.isTrainerValidForPlayer(uint32(tType), uint32(tReq)) {
			_ = s.write(uint16(protocol.OpcodeSMSG_TRAINER_BUY_FAILED), buildTrainerBuyFailed(trainerGUID, spellID, 2), true)
			return true
		}
	}

	// 2. Fetch spell requirements on this trainer
	var moneyCost, reqLevel, reqSkill, reqSkillValue, reqAb1, reqAb2, reqAb3 int64
	var foundSpell bool
	if foundTrainer {
		err = wdb.QueryRowContext(ctx, `SELECT COALESCE(MoneyCost, 0), COALESCE(ReqLevel, 0),
			COALESCE(ReqSkillLine, 0), COALESCE(ReqSkillRank, 0),
			COALESCE(ReqAbility1, 0), COALESCE(ReqAbility2, 0), COALESCE(ReqAbility3, 0)
			FROM trainer_spell WHERE TrainerId = ? AND SpellId = ? LIMIT 1`,
			trainerID, spellID).Scan(&moneyCost, &reqLevel, &reqSkill, &reqSkillValue, &reqAb1, &reqAb2, &reqAb3)
		if err == nil {
			foundSpell = true
		}
	}

	// Fallback for legacy npc_trainer or tests where creature has no trainer row
	if !foundSpell && !foundTrainer {
		err = wdb.QueryRowContext(ctx, `SELECT COALESCE(MoneyCost, 0), COALESCE(ReqLevel, 0),
			COALESCE(ReqSkill, 0), COALESCE(ReqSkillValue, 0), 0, 0, 0
			FROM npc_trainer WHERE ID = ? AND SpellID = ? LIMIT 1`,
			creatureEntry, spellID).Scan(&moneyCost, &reqLevel, &reqSkill, &reqSkillValue, &reqAb1, &reqAb2, &reqAb3)
		if err == nil {
			foundSpell = true
		}
	}

	// Test fallback: if trainer row was NOT found at all, check class fallback
	if !foundSpell && !foundTrainer && s.player != nil && s.player.Class > 0 {
		err = wdb.QueryRowContext(ctx, `SELECT COALESCE(ts.MoneyCost, 0), COALESCE(ts.ReqLevel, 0),
			COALESCE(ts.ReqSkillLine, 0), COALESCE(ts.ReqSkillRank, 0),
			COALESCE(ts.ReqAbility1, 0), COALESCE(ts.ReqAbility2, 0), COALESCE(ts.ReqAbility3, 0)
			FROM trainer_spell AS ts
			JOIN trainer AS t ON t.Id = ts.TrainerId
			WHERE t.Type = 0 AND t.Requirement = ? AND ts.SpellId = ? LIMIT 1`,
			s.player.Class, spellID).Scan(&moneyCost, &reqLevel, &reqSkill, &reqSkillValue, &reqAb1, &reqAb2, &reqAb3)
		if err == nil {
			foundSpell = true
		}
	}

	if !foundSpell {
		_ = s.write(uint16(protocol.OpcodeSMSG_TRAINER_BUY_FAILED), buildTrainerBuyFailed(trainerGUID, spellID, 2), true)
		s.debug("trainer spell not found", "account", s.accountName, "trainer", trainerGUID, "entry", creatureEntry, "spell", spellID)
		return true
	}

	// 3. Class & Race check
	if !s.isSpellFitByClassAndRace(spellID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_TRAINER_BUY_FAILED), buildTrainerBuyFailed(trainerGUID, spellID, 2), true)
		return true
	}

	// 4. Validate TrainerSpellState (must be 0 / Available)
	reqAbilities := []uint32{uint32(reqAb1), uint32(reqAb2), uint32(reqAb3)}
	state := s.getTrainerSpellState(spellID, uint8(reqLevel), uint32(reqSkill), uint32(reqSkillValue), reqAbilities)
	if state != 0 {
		reason := uint32(2) // NotEnoughSkill = 2
		if state == 2 {
			reason = 0 // AlreadyKnown = 0
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_TRAINER_BUY_FAILED), buildTrainerBuyFailed(trainerGUID, spellID, reason), true)
		return true
	}

	// 5. Money check
	if s.player.Money < uint32(moneyCost) {
		_ = s.write(uint16(protocol.OpcodeSMSG_TRAINER_BUY_FAILED), buildTrainerBuyFailed(trainerGUID, spellID, 1), true) // NotEnoughMoney = 1
		return true
	}

	// 6. Deduct money & update DB
	s.player.Money -= uint32(moneyCost)
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)

	// 7. Play visual on trainer NPC (TC: npc->SendPlaySpellVisual(179))
	visualBuf := protocol.NewBuffer(12)
	visualBuf.WriteU64(trainerGUID)
	visualBuf.WriteU32(179) // SpellVisualKit 179: Trainer Teach
	_ = s.write(uint16(protocol.OpcodeSMSG_PLAY_SPELL_VISUAL), visualBuf.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PLAY_SPELL_VISUAL), visualBuf.Bytes(), s)
	}

	// 8. Play impact on player with sound chime (TC: npc->SendPlaySpellImpact(player->GetGUID(), 362))
	impactBuf := protocol.NewBuffer(12)
	impactBuf.WriteU64(s.playerGUID)
	impactBuf.WriteU32(362) // SpellVisualKit 362: Spell Learn Chime & Impact
	_ = s.write(uint16(protocol.OpcodeSMSG_PLAY_SPELL_IMPACT), impactBuf.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PLAY_SPELL_IMPACT), impactBuf.Bytes(), s)
	}

	// 9. Check if trainerSpell is castable (HasEffect SPELL_EFFECT_LEARN_SPELL = 36)
	isCastable := false
	var triggerSpell uint32
	if s.server != nil && s.server.Data != nil {
		if sp, ok, err := s.server.Data.Spell(spellID); err == nil && ok {
			for _, eff := range sp.Effects {
				if eff.Effect == 36 { // SPELL_EFFECT_LEARN_SPELL
					isCastable = true
					triggerSpell = eff.TriggerSpell
					break
				}
			}
		}
	}

	if isCastable {
		// Player casts the teach spell (TC: player->CastSpell(player, trainerSpell->SpellId, true))
		castTimeStamp := uint32(time.Now().UnixMilli())
		target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: s.playerGUID}
		goPkt := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, 1, spellID, spellCastFlagGo, castTimeStamp, []uint64{s.playerGUID}, nil, target)
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, s)
		}
		if triggerSpell > 0 {
			s.learnSpell(ctx, triggerSpell)
		} else {
			s.learnSpell(ctx, spellID)
		}
	} else {
		s.learnSpell(ctx, spellID)
	}

	// 11. Check spell_learn_spell for dependent spells (e.g. Find Herbs, Smelting, Campfire, etc.)
	var depSpells []uint32
	depRows, dErr := wdb.QueryContext(ctx, "SELECT SpellID FROM spell_learn_spell WHERE entry = ?", spellID)
	if dErr == nil {
		for depRows.Next() {
			var depSpell uint32
			if err := depRows.Scan(&depSpell); err == nil && depSpell > 0 {
				depSpells = append(depSpells, depSpell)
			}
		}
		_ = depRows.Close()
	}
	for _, depSpell := range depSpells {
		if !s.hasLearnedSpell(depSpell) {
			s.learnSpell(ctx, depSpell)
		}
	}

	// 12. Send teach succeeded (TC: SendTeachSucceeded(npc, player, spellId))
	_ = s.write(uint16(protocol.OpcodeSMSG_TRAINER_BUY_SUCCEEDED), buildTrainerBuySucceeded(trainerGUID, spellID), true)
	s.sendPlayerUpdate()
	s.debug("trainer spell learned", "account", s.accountName, "spell", spellID, "cost", moneyCost)
	return true
}

func (s *session) learnSpell(ctx context.Context, spellID uint32) {
	if s.hasLearnedSpell(spellID) {
		return
	}
	cdb := s.server.CharactersStore.DB

	// Check if this spell supercedes an earlier rank (TrinityCore: Player::SendSupercededSpell)
	var prevSpellID uint32
	if s.server != nil {
		prevSpellID = s.server.getPrevSpellInChain(spellID)
	}
	if prevSpellID == 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT r1.spell_id FROM spell_ranks AS r1
			JOIN spell_ranks AS r2 ON r1.first_spell_id = r2.first_spell_id
			WHERE r2.spell_id = ? AND r1.rank = r2.rank - 1 LIMIT 1`, spellID).Scan(&prevSpellID)
	}
	if prevSpellID > 0 && s.hasLearnedSpell(prevSpellID) {
		if cdb != nil {
			_, _ = cdb.ExecContext(ctx, "UPDATE character_spell SET active = 0 WHERE guid = ? AND spell = ?", s.playerGUID, prevSpellID)
		}
		if s.hasAura(prevSpellID) {
			s.removeAura(prevSpellID)
		}
		s.removeOwnerPetAurasForSpell(ctx, prevSpellID)
		for i := range s.player.Spells {
			if s.player.Spells[i].ID == prevSpellID {
				s.player.Spells[i].Active = false
			}
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_SUPERCEDED_SPELL), buildSupercededSpell(prevSpellID, spellID), true)
	}

	if cdb != nil {
		_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", s.playerGUID, spellID)
	}
	s.learnOwnerPetAuraSources(ctx, spellID)
	s.player.Spells = append(s.player.Spells, learnedSpell{ID: spellID, Active: true, Disabled: false})

	learnedBuf := protocol.NewBuffer(6)
	learnedBuf.WriteU32(spellID)
	learnedBuf.WriteU16(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_LEARNED_SPELL), learnedBuf.Bytes(), true)

	// Yellow chat notification: "You have learned a new spell: <Name>."
	if s.server != nil && s.server.Data != nil {
		if name, rank, ok, err := s.server.Data.SpellName(spellID); err == nil && ok && name != "" {
			msg := "You have learned a new spell: " + name
			if rank != "" {
				msg += " (" + rank + ")"
			}
			msg += "."
			s.sendSystemMessage(msg)
		}
	}

	// Check if passive spell (TC: if (spellInfo->IsPassive()) CastSpell(this, spellId, true))
	if s.server != nil && s.server.Data != nil {
		if sp, ok, err := s.server.Data.Spell(spellID); err == nil && ok {
			if sp.Attributes&0x40 != 0 { // SPELL_ATTR0_PASSIVE
				castTimeStamp := uint32(time.Now().UnixMilli())
				target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: s.playerGUID}
				goPkt := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, 1, spellID, spellCastFlagGo, castTimeStamp, []uint64{s.playerGUID}, nil, target)
				_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
				if s.server != nil {
					s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, s)
				}
				if s.auras == nil {
					s.auras = make(map[uint32]struct{})
				}
				s.auras[spellID] = struct{}{}
			}
		}
	}

	// Check if spell teaches or upgrades a skill (SPELL_EFFECT_LEARN_SKILL = 118)
	if s.server != nil && s.server.Data != nil {
		if sp, ok, err := s.server.Data.Spell(spellID); err == nil && ok {
			for _, eff := range sp.Effects {
				if eff.Effect == 118 && eff.MiscValue > 0 { // SPELL_EFFECT_LEARN_SKILL
					skillID := uint16(eff.MiscValue)
					tierIdx := eff.BasePoints + 1 // 1=Apprentice, 2=Journeyman, 3=Expert, 4=Artisan, 5=Master, 6=Grand Master
					var maxVal uint16
					switch tierIdx {
					case 1:
						maxVal = 75
					case 2:
						maxVal = 150
					case 3:
						maxVal = 225
					case 4:
						maxVal = 300
					case 5:
						maxVal = 375
					case 6:
						maxVal = 450
					default:
						if tierIdx > 0 {
							maxVal = uint16(tierIdx * 75)
						} else {
							maxVal = 75
						}
					}
					s.setOrUpdateSkill(ctx, skillID, maxVal)
				}
			}
		}
	}
}

func (s *session) setOrUpdateSkill(ctx context.Context, skillID, maxVal uint16) {
	if s.player == nil {
		return
	}
	found := false
	for i := range s.player.Skills {
		if s.player.Skills[i].Skill == skillID {
			found = true
			if s.player.Skills[i].Max < maxVal {
				s.player.Skills[i].Max = maxVal
			}
			if s.player.Skills[i].Value < 1 {
				s.player.Skills[i].Value = 1
			}
			if skillID == 762 { // Riding reaches max value immediately
				s.player.Skills[i].Value = maxVal
			}
			if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_skills SET value = ?, max = ? WHERE guid = ? AND skill = ?", s.player.Skills[i].Value, s.player.Skills[i].Max, s.playerGUID, skillID)
			}
			s.setAchievementCriteria(criteriaTypeLearnSkillLevel, uint32(skillID), uint32(maxVal))
			break
		}
	}
	if !found {
		val := uint16(1)
		if skillID == 762 { // Riding starts at max
			val = maxVal
		}
		newSk := playerSkill{
			Skill: skillID,
			Step:  1,
			Value: val,
			Max:   maxVal,
			Bonus: 0,
		}
		s.player.Skills = append(s.player.Skills, newSk)
		s.updateAchievementCriteria(criteriaTypeLearnSkillLine, uint32(skillID), 1)
		s.updateAchievementCriteria(criteriaTypeLearnSkillLineSpells, uint32(skillID), 1)
		s.setAchievementCriteria(criteriaTypeLearnSkillLevel, uint32(skillID), uint32(maxVal))
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, ?, ?)", s.playerGUID, skillID, val, maxVal)
		}
	}
}

func (s *session) getSkillValue(skillID uint32) uint32 {
	if s.player == nil {
		return 0
	}
	for _, sk := range s.player.Skills {
		if uint32(sk.Skill) == skillID {
			return uint32(sk.Value)
		}
	}
	return 0
}

func (s *session) hasLearnedSpell(spellID uint32) bool {
	for _, sp := range s.player.Spells {
		if sp.ID == spellID {
			return true
		}
	}
	return false
}

func buildTrainerBuySucceeded(trainerGUID uint64, spellID uint32) []byte {
	buf := protocol.NewBuffer(12)
	buf.WriteU64(trainerGUID)
	buf.WriteU32(spellID)
	return buf.Bytes()
}

func buildTrainerBuyFailed(trainerGUID uint64, spellID, reason uint32) []byte {
	buf := protocol.NewBuffer(16)
	buf.WriteU64(trainerGUID)
	buf.WriteU32(spellID)
	buf.WriteU32(reason)
	return buf.Bytes()
}

func buildPlaySound(soundID uint32) []byte {
	buf := protocol.NewBuffer(4)
	buf.WriteU32(soundID)
	return buf.Bytes()
}

func buildSupercededSpell(oldSpell, newSpell uint32) []byte {
	buf := protocol.NewBuffer(8)
	buf.WriteU32(oldSpell)
	buf.WriteU32(newSpell)
	return buf.Bytes()
}

func (s *Server) getPrevSpellInChain(spellID uint32) uint32 {
	if s == nil {
		return 0
	}
	s.spellChainMu.RLock()
	if s.spellChainLoaded {
		prev := s.prevSpellInChain[spellID]
		s.spellChainMu.RUnlock()
		return prev
	}
	s.spellChainMu.RUnlock()

	s.spellChainMu.Lock()
	defer s.spellChainMu.Unlock()
	if s.spellChainLoaded {
		return s.prevSpellInChain[spellID]
	}
	s.prevSpellInChain = make(map[uint32]uint32)
	s.spellChainLoaded = true
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		rows, err := s.WorldStore.DB.Query(`SELECT r2.spell_id, r1.spell_id
			FROM spell_ranks AS r1
			JOIN spell_ranks AS r2 ON r1.first_spell_id = r2.first_spell_id AND r1.rank = r2.rank - 1`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var higherSpell, prevSpell uint32
				if err := rows.Scan(&higherSpell, &prevSpell); err == nil && higherSpell > 0 && prevSpell > 0 {
					s.prevSpellInChain[higherSpell] = prevSpell
				}
			}
		}
	}
	return s.prevSpellInChain[spellID]
}

func (s *session) isTrainerValidForPlayer(tType, requirement uint32) bool {
	if s.player == nil {
		return false
	}
	switch tType {
	case 0, 3: // TRAINER_TYPE_CLASS, TRAINER_TYPE_PETS
		if requirement != 0 && requirement != uint32(s.player.Class) {
			return false
		}
	case 1: // TRAINER_TYPE_MOUNTS
		if requirement != 0 && requirement != uint32(s.player.Race) {
			return false
		}
	case 2: // TRAINER_TYPE_TRADESKILLS
		if requirement != 0 && !s.hasLearnedSpell(requirement) {
			return false
		}
	}
	return true
}

func (s *session) isSpellFitByClassAndRace(spellID uint32) bool {
	if s.player == nil {
		return false
	}
	if s.server == nil || s.server.Data == nil {
		return true
	}
	abilities, ok, err := s.server.Data.SkillLineAbilities(spellID)
	if err != nil || !ok || len(abilities) == 0 {
		return true
	}

	raceMask := uint32(0)
	if s.player.Race > 0 && s.player.Race <= 32 {
		raceMask = uint32(1) << (s.player.Race - 1)
	}
	classMask := uint32(0)
	if s.player.Class > 0 && s.player.Class <= 32 {
		classMask = uint32(1) << (s.player.Class - 1)
	}

	for _, entry := range abilities {
		if entry.RaceMask != 0 && raceMask != 0 && (entry.RaceMask&raceMask) == 0 {
			continue
		}
		if entry.ClassMask != 0 && classMask != 0 && (entry.ClassMask&classMask) == 0 {
			continue
		}
		return true
	}
	return false
}

func (s *session) getTrainerSpellState(spellID uint32, reqLevel uint8, reqSkill, reqSkillValue uint32, reqAbilities []uint32) uint8 {
	if s.player == nil {
		return 1 // Unavailable
	}

	// 1. Already known: Green (2)
	if s.hasLearnedSpell(spellID) {
		return 2
	}

	// 2. Check previous rank in chain (TrinityCore: GetPrevSpellInChain)
	if s.server != nil {
		if prevSpell := s.server.getPrevSpellInChain(spellID); prevSpell > 0 && !s.hasLearnedSpell(prevSpell) {
			return 1 // Unavailable
		}
	}

	// 3. Check player level
	if s.player.Level < reqLevel {
		return 1 // Unavailable
	}

	// 4. Check required skill
	if reqSkill > 0 && s.getSkillValue(reqSkill) < reqSkillValue {
		return 1 // Unavailable
	}

	// 5. Check required abilities
	for _, reqAb := range reqAbilities {
		if reqAb > 0 && !s.hasLearnedSpell(reqAb) {
			return 1 // Unavailable
		}
	}

	return 0 // Available
}

func (s *session) sendSystemMessage(message string) {
	_ = s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), protocol.BuildSystemChatMessage(message), true)
}
