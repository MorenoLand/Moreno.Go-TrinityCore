package world

import (
	"context"
	"database/sql"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func totalReputationStanding(reputation playerReputation) int32 {
	return reputation.Base + reputation.Standing
}

// reputationAutoWar mirrors the SetOneFactionReputation hostile arm
// (ReputationMgr.cpp:403) plus the SetAtWar peace-forced gate (:478-484):
// a new total rank at or below HOSTILE declares war unless the faction is
// peace-forced, non-rival, and its RAW standing ranks above HATED. The
// pointer overload carries no invisible/hidden check, unlike the load fold.
func reputationAutoWar(flags uint8, totalStanding, rawStanding int32) bool {
	return reputationRank(int64(totalStanding)) <= 1 && !(flags&factionFlagPeaceForced != 0 && flags&factionFlagRival == 0 && reputationRank(int64(rawStanding)) > 0)
}

// reputationStanding mirrors ReputationMgr::GetReputation
// (ReputationMgr.cpp:119): base reputation plus recorded standing for the
// faction, 0 when the faction has no recorded state.
func (s *session) reputationStanding(factionID uint32) int32 {
	if s == nil || s.player == nil || factionID == 0 {
		return 0
	}
	for i := range s.player.Reputations {
		if s.player.Reputations[i].FactionID == factionID {
			return totalReputationStanding(s.player.Reputations[i])
		}
	}
	return 0
}

const (
	factionFlagAtWar           uint8 = 0x02
	factionFlagHidden          uint8 = 0x04
	factionFlagInvisibleForced uint8 = 0x08
	factionFlagPeaceForced     uint8 = 0x10
	factionFlagInactive        uint8 = 0x20
	factionFlagRival           uint8 = 0x40
	factionFlagSpecial         uint8 = 0x80
	factionFlagVisible         uint8 = 0x01
)

const (
	spellAuraModReputationGain        = 156 // SPELL_AURA_MOD_REPUTATION_GAIN (SpellAuraDefines.h:236)
	spellAuraModFactionReputationGain = 190 // SPELL_AURA_MOD_FACTION_REPUTATION_GAIN (SpellAuraDefines.h:270)
)

// reputationGainPctMod mirrors Player::CalculateReputationGain's aura fold
// (Player.cpp:6653-6659): the live 156 modifier plus, when
// includeFactionSpecific is set, the faction-specific 190 modifier whose
// effect MiscValue names this faction. C++ gates 190 to kills
// ("faction specific auras only seem to apply to kills"); spell and quest
// sources fold 156 only, while the battleground team-reward arm applies
// both (Battleground.cpp:640-658, AddPct twice). C++ applies it as
// percent = 100 + repMod for gains, 100 - repMod for losses (CalculatePct
// at the end). The low-level rate and RepRewardRate legs live in the
// per-source calculators (kill, quest, spell), not here: this fold is the
// aura leg only, shared by all of them plus the battleground arm.
func (s *session) reputationGainPctMod(factionID uint32, includeFactionSpecific bool) (mod int32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			switch effect.Aura {
			case spellAuraModReputationGain:
				mod += aura.Amounts[index]
			case spellAuraModFactionReputationGain:
				if includeFactionSpecific && uint32(effect.MiscValue) == factionID {
					mod += aura.Amounts[index]
				}
			}
		}
	}
	return mod
}

// MergeReputationFlags replays ReputationMgr::LoadFromDB's flag fold
// (ReputationMgr.cpp:540-573) over the DB flags. totalStanding is the
// client-visible total (base + raw); rawStanding is the raw
// FactionState::Standing (DB column, base excluded). The peace-forced
// war-declaration gate (SetAtWar, ReputationMgr.cpp:478-484) ranks the RAW
// standing, while the hostile auto-war arm ranks the total via GetRank —
// the two gates must not share one rank input.
func MergeReputationFlags(defaultFlags, databaseFlags uint8, totalStanding, rawStanding int32) uint8 {
	flags := defaultFlags
	if databaseFlags&factionFlagVisible != 0 && (flags&(factionFlagInvisibleForced|factionFlagHidden) == 0 || flags&factionFlagSpecial != 0) {
		flags |= factionFlagVisible
	}
	if databaseFlags&factionFlagInactive != 0 && flags&(factionFlagInvisibleForced|factionFlagHidden) == 0 && flags&factionFlagVisible != 0 {
		flags |= factionFlagInactive
	}
	rawRank := reputationRank(int64(rawStanding))
	canChangeWar := flags&(factionFlagInvisibleForced|factionFlagHidden) == 0
	canDeclareWar := flags&factionFlagPeaceForced == 0 || flags&factionFlagRival != 0 || rawRank == 0
	if databaseFlags&factionFlagAtWar != 0 && canChangeWar && canDeclareWar {
		flags |= factionFlagAtWar
	} else if databaseFlags&factionFlagAtWar == 0 && flags&factionFlagVisible != 0 && canChangeWar {
		flags &^= factionFlagAtWar
	}
	if reputationRank(int64(totalStanding)) <= 1 && canChangeWar && canDeclareWar {
		flags |= factionFlagAtWar
	}
	return flags
}

func (s *session) applyStartAllReputation(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	factions := []uint32{942, 935, 936, 1011, 970, 967, 989, 932, 934, 1038, 1077, 1106, 1104, 1090, 1098, 1156, 1073, 1105, 1119, 1091}
	if teamForRace(s.player.Race) == 0 {
		factions = append(factions, 72, 47, 69, 930, 730, 978, 54, 946, 1037, 1068, 1126, 1094, 1050)
	} else {
		factions = append(factions, 76, 68, 81, 911, 729, 941, 530, 947, 1052, 1067, 1124, 1064, 1085)
	}
	changed := make([]playerReputation, 0, len(factions))
	increased := false
	for _, factionID := range factions {
		reputation, found, err := s.server.Data.Reputation(factionID, s.player.Race, s.player.Class)
		if err != nil || !found || reputation.ReputationList < 0 {
			continue
		}
		index := -1
		for i := range s.player.Reputations {
			if s.player.Reputations[i].FactionID == factionID {
				index = i
				break
			}
		}
		if index < 0 {
			s.player.Reputations = append(s.player.Reputations, playerReputation{FactionID: factionID, ListID: uint32(reputation.ReputationList), Base: reputation.BaseStanding, Flags: reputation.DefaultFlags})
			index = len(s.player.Reputations) - 1
		}
		oldRank := reputationRank(int64(totalReputationStanding(s.player.Reputations[index])))
		s.player.Reputations[index].ListID = uint32(reputation.ReputationList)
		s.player.Reputations[index].Standing = 42999 - reputation.BaseStanding
		s.player.Reputations[index].Flags |= factionFlagVisible
		if s.player.Reputations[index].Flags == factionFlagVisible {
			s.player.Reputations[index].Flags |= reputation.DefaultFlags
		}
		if reputationRank(int64(totalReputationStanding(s.player.Reputations[index]))) > oldRank {
			increased = true
		}
		changed = append(changed, s.player.Reputations[index])
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_reputation (guid, faction, standing, flags) VALUES (?, ?, ?, ?)", s.playerGUID, factionID, s.player.Reputations[index].Standing, s.player.Reputations[index].Flags)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState(changed, increased), true)
}

func buildFactionStandingState(reputations []playerReputation, increased bool) []byte {
	packet := protocol.NewBuffer(13 + len(reputations)*8)
	packet.WriteF32(0)
	if increased {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU32(uint32(len(reputations)))
	for _, reputation := range reputations {
		packet.WriteU32(reputation.ListID)
		packet.WriteU32(uint32(reputation.Standing))
	}
	return packet.Bytes()
}

// handleSetWatchedFaction mirrors WorldSession::HandleSetWatchedFactionOpcode
// (CharacterHandler.cpp): read the reputation list index and store it in
// PLAYER_FIELD_WATCHED_FACTION_INDEX, pushing the field change to the owning
// client as a values update.
func (s *session) handleSetWatchedFaction(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	reader := protocol.NewReader(payload)
	fact, err := reader.ReadU32()
	if err != nil {
		return true // truncated: drop, do not close
	}
	s.player.WatchedFaction = fact
	packet, err := s.server.buildPlayerValuesUpdate(s.playerGUID, map[int]uint32{unitFieldWatchedFaction: fact})
	if err != nil {
		s.debug("watched faction values update build failed", "account", s.accountName, "error", err)
		return true
	}
	_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
	return true
}

// handleSetFactionInactive mirrors WorldSession::HandleSetFactionInactiveOpcode
// plus ReputationMgr::SetInactive: toggle FACTION_FLAG_INACTIVE on the faction
// matching the reputation list ID. Hidden, forced-invisible, or not-yet-visible
// factions cannot be inactivated, and already-matching states are ignored. The
// flag is persisted immediately like the at-war toggle and only reaches the
// client through SMSG_INITIALIZE_FACTIONS on the next login, matching the
// reference where SendState carries standing but never flags.
func (s *session) handleSetFactionInactive(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 5 {
		return true
	}
	reader := protocol.NewReader(payload)
	listID, err := reader.ReadU32()
	if err != nil {
		return true
	}
	inactive, err := reader.ReadU8()
	if err != nil {
		return true
	}
	for index := range s.player.Reputations {
		reputation := &s.player.Reputations[index]
		if reputation.ListID != listID {
			continue
		}
		if inactive != 0 {
			if reputation.Flags&(factionFlagInvisibleForced|factionFlagHidden) != 0 || reputation.Flags&factionFlagVisible == 0 {
				return true
			}
			if reputation.Flags&factionFlagInactive != 0 {
				return true
			}
			reputation.Flags |= factionFlagInactive
		} else {
			if reputation.Flags&factionFlagInactive == 0 {
				return true
			}
			reputation.Flags &^= factionFlagInactive
		}
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_reputation SET flags = ? WHERE guid = ? AND faction = ?", reputation.Flags, s.playerGUID, reputation.FactionID)
		}
		// ReputationMgr::SetInactive (ReputationMgr.cpp:521): the flag change
		// sets needSend, flushed immediately as SMSG_SET_FACTION_STANDING
		// (SendState, ReputationMgr.cpp:184-209).
		_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState([]playerReputation{*reputation}, false), true)
		s.debug("faction inactive state changed", "account", s.accountName, "faction", reputation.FactionID, "inactive", inactive != 0)
		return true
	}
	return true
}

func (s *session) handleSetFactionAtWar(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 5 {
		return true
	}
	reader := protocol.NewReader(payload)
	listID, err := reader.ReadU32()
	if err != nil {
		return true
	}
	flag, err := reader.ReadU8()
	if err != nil {
		return true
	}
	for index := range s.player.Reputations {
		reputation := &s.player.Reputations[index]
		if reputation.ListID != listID {
			continue
		}
		// ReputationMgr::SetAtWar (ReputationMgr.cpp:465-495): always-invisible
		// or hidden factions cannot change war state; war cannot be declared on
		// our own faction (peace-forced, non-rival, standing above HATED); an
		// already-set state is a no-op.
		if reputation.Flags&(factionFlagInvisibleForced|factionFlagHidden) != 0 {
			return true
		}
		if flag != 0 {
			if reputation.Flags&factionFlagPeaceForced != 0 && reputation.Flags&factionFlagRival == 0 &&
				reputationRank(int64(reputation.Standing)) > 0 {
				return true
			}
			if reputation.Flags&factionFlagAtWar != 0 {
				return true
			}
			reputation.Flags |= factionFlagAtWar
		} else {
			if reputation.Flags&factionFlagAtWar == 0 {
				return true
			}
			reputation.Flags &^= factionFlagAtWar
		}
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_reputation SET flags = ? WHERE guid = ? AND faction = ?", reputation.Flags, s.playerGUID, reputation.FactionID)
		}
		// ReputationMgr::SetAtWar (ReputationMgr.cpp:493): the flag change sets
		// needSend, flushed immediately as SMSG_SET_FACTION_STANDING.
		_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState([]playerReputation{*reputation}, false), true)
		s.debug("faction war state changed", "account", s.accountName, "faction", reputation.FactionID, "at_war", flag != 0)
		return true
	}
	return true
}

// sendFactionVisiblePacket mirrors ReputationMgr::SendVisible
// (ReputationMgr.cpp:445-460): SMSG_SET_FACTION_VISIBLE carries the
// faction's ReputationList id. Called on the SetVisible transition inside
// SetOneFactionReputation, before the standing packet SendState sends.
func (s *session) sendFactionVisiblePacket(listID uint32) {
	packet := protocol.NewBuffer(4)
	packet.WriteU32(listID)
	_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_VISIBLE), packet.Bytes(), true)
}

// factionVisibleGated mirrors the SetVisible gate (ReputationMgr.cpp:445-460):
// always-invisible or hidden factions can never be made visible, except
// when flagged SPECIAL.
func factionVisibleGated(flags uint8) bool {
	return flags&(factionFlagInvisibleForced|factionFlagHidden) != 0 && flags&factionFlagSpecial == 0
}

// fireReputationCriteria mirrors the criteria arms of
// SetOneFactionReputation (ReputationMgr.cpp:405-410) as dispatched by
// AchievementMgr::UpdateAchievementCriteria: GAIN_REPUTATION sets the
// faction's standing when positive, while the honored/revered/exalted arms
// set the absolute counts of factions at each rank and KNOWN_FACTIONS
// sets the absolute visible-faction count (AchievementMgr.cpp:967-1020).
// The counts are absolute, not per-gain increments: C++ SetCriteriaProgress
// rewrites the count, so repeated gains at the same rank must not inflate it.
func (s *session) fireReputationCriteria(factionID uint32, totalStanding int32) {
	if s == nil || s.player == nil {
		return
	}
	if totalStanding > 0 {
		s.setAchievementCriteria(criteriaTypeGainReputation, factionID, uint32(totalStanding))
	}
	var honored, revered, exalted, visible uint32
	for _, rep := range s.player.Reputations {
		total := totalReputationStanding(rep)
		if rep.Flags&factionFlagVisible != 0 {
			visible++
		}
		if total >= 9000 {
			honored++
		}
		if total >= 21000 {
			revered++
		}
		if total >= 42000 {
			exalted++
		}
	}
	s.setAchievementCriteria(criteriaTypeHonoredRep, 0, honored)
	s.setAchievementCriteria(criteriaTypeReveredRep, 0, revered)
	s.setAchievementCriteria(criteriaTypeExaltedRep, 0, exalted)
	s.setAchievementCriteria(criteriaTypeKnownFactions, 0, visible)
}

func (s *session) giveReputation(ctx context.Context, factionID uint32, amount int32, includeFactionSpecific bool) {
	if s.player == nil || factionID == 0 || amount == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	// Player::CalculateReputationGain (Player.cpp:6657): percent += rep > 0 ?
	// repMod : -repMod, then CalculatePct(rep, percent); a zero or negative
	// percent grants nothing. This is the battleground arm's shape
	// (Battleground::RewardReputationToTeam adds both aura modifiers, then
	// ModifyReputation directly): no low-level rate, no reward-rate row,
	// no recruit-a-friend fold. Spell-effect gains use
	// giveSpellReputation, which adds the spell-rate leg.
	if mod := s.reputationGainPctMod(factionID, includeFactionSpecific); mod != 0 {
		percent := int64(100 + mod)
		if amount < 0 {
			percent = int64(100 - mod)
		}
		if percent <= 0 {
			return
		}
		amount = int32(int64(amount) * percent / 100)
		if amount == 0 {
			return
		}
	}
	s.addReputationAmount(ctx, factionID, amount)
}

// calculateSpellReputationGain mirrors Player::CalculateReputationGain
// (Player.cpp:6651-6726) for REPUTATION_SOURCE_SPELL: the 156 aura modifier
// folds into percent (the kill-only 190 modifier never does), the low-level
// rate is 1.0 for this source, a zero or negative percent grants nothing,
// the spell_rate column folds next (a zero rate disables the gain), and
// the recruit-a-friend bonus never folds for spells.
func (s *session) calculateSpellReputationGain(ctx context.Context, factionID uint32, rep int32) int32 {
	percent := float32(100)
	if mod := s.reputationGainPctMod(factionID, false); mod != 0 {
		if rep > 0 {
			percent += float32(mod)
		} else {
			percent -= float32(mod)
		}
	}
	if percent <= 0 {
		return 0
	}
	var ok bool
	if percent, ok = s.applyRepRewardRate(ctx, factionID, repSourceSpell, percent); !ok {
		return 0
	}
	return int32(float32(rep) * percent / 100)
}

// giveSpellReputation is the Spell::EffectReputation sink
// (SpellEffects.cpp:4349-4368): CalculateReputationGain with the spell
// source, then ModifyReputation through the shared incremental sink.
func (s *session) giveSpellReputation(ctx context.Context, factionID uint32, amount int32) {
	if s.player == nil || factionID == 0 || amount == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	amount = s.calculateSpellReputationGain(ctx, factionID, amount)
	if amount == 0 {
		return
	}
	s.addReputationAmount(ctx, factionID, amount)
}

// addReputationAmount is the hook+persist sink shared by giveReputation and
// the quest-reward pipeline (which scales the amount itself, mirroring
// Player::RewardReputation + CalculateReputationGain).
// ReputationMgr::SetReputation (ReputationMgr.cpp:302): the Eluna hook
// fires first with the target standing; a rewritten standing becomes the
// new target, folded back to a delta for the additive store. Go's sink
// is always incremental. The GM .modify rep path mirrors C++
// SetOneFactionReputation, which never fires the hook (only SetReputation
// does), so it stays unfired there.
func (s *session) addReputationAmount(ctx context.Context, factionID uint32, amount int32) {
	if s.player == nil || factionID == 0 || amount == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	currentTotal := s.reputationStanding(factionID)
	target := currentTotal + amount
	target = s.fireReputationChangeHook(ctx, factionID, target, true)
	amount = target - currentTotal
	// ReputationMgr::SetReputation (ReputationMgr.cpp:302-358): spillover
	// runs before the main-faction write, on the hook-rewritten delta, even
	// when that delta is zero (the DB-template arm still writes its slots).
	s.applyReputationSpillover(ctx, factionID, amount, true)
	if amount == 0 {
		return
	}
	cdb := s.server.CharactersStore.DB
	for i := range s.player.Reputations {
		if s.player.Reputations[i].FactionID == factionID {
			oldRank := reputationRank(int64(totalReputationStanding(s.player.Reputations[i])))
			// SetOneFactionReputation (ReputationMgr.cpp:374): the total
			// clamps to [-42000, 42999] and stores net of the base.
			newTotal := currentTotal + amount
			if newTotal > 42999 {
				newTotal = 42999
			} else if newTotal < -42000 {
				newTotal = -42000
			}
			s.player.Reputations[i].Standing = newTotal - s.player.Reputations[i].Base
			// SetOneFactionReputation (ReputationMgr.cpp:401) calls
			// SetVisible: a gain makes a non-visible faction visible
			// (gated factions excepted), announced before the standing
			// packet. The load seeds every DBC faction row, so this
			// existing-row arm is the common first-gain path, not the
			// first-seen arm below.
			becameVisible := false
			if !factionVisibleGated(s.player.Reputations[i].Flags) && s.player.Reputations[i].Flags&factionFlagVisible == 0 {
				s.player.Reputations[i].Flags |= factionFlagVisible
				becameVisible = true
			}
			if reputationAutoWar(s.player.Reputations[i].Flags, newTotal, s.player.Reputations[i].Standing) {
				s.player.Reputations[i].Flags |= factionFlagAtWar
			}
			_, _ = cdb.ExecContext(ctx, "UPDATE character_reputation SET standing = ?, flags = ? WHERE guid = ? AND faction = ?", s.player.Reputations[i].Standing, s.player.Reputations[i].Flags, s.playerGUID, factionID)
			totalStanding := totalReputationStanding(s.player.Reputations[i])
			if becameVisible {
				s.sendFactionVisiblePacket(s.player.Reputations[i].ListID)
			}
			// ReputationMgr::SetReputation (ReputationMgr.cpp:369): the changed
			// faction is reported to the client immediately via SendState as
			// SMSG_SET_FACTION_STANDING; _sendFactionIncreased is the rank
			// step-up (ReputationMgr.cpp:405-406).
			_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState([]playerReputation{s.player.Reputations[i]}, reputationRank(int64(totalStanding)) > oldRank), true)
			s.fireReputationCriteria(factionID, totalStanding)
			// Player::SetReputation (ReputationMgr.cpp:302) runs the quest
			// ReputationChanged scan after the standing lands.
			s.questReputationChanged(ctx, factionID)
			return
		}
	}
	// A first-seen faction starts from its DBC row (ListID, base, default
	// flags) like the GM path, not a ListID-0 phantom: the total clamps,
	// SetVisible applies (ReputationMgr.cpp:445-458), and the hostile arm
	// can declare war. A DBC miss keeps the legacy phantom row rather than
	// dropping the gain outright.
	rep := playerReputation{
		FactionID: factionID,
		Standing:  amount,
		Flags:     factionFlagVisible,
	}
	dbcBacked := false
	becameVisible := false
	oldRank := reputationRank(0)
	if s.server != nil && s.server.Data != nil {
		if dbcRep, found, dbcErr := s.server.Data.Reputation(factionID, s.player.Race, s.player.Class); dbcErr == nil && found && dbcRep.ReputationList >= 0 {
			dbcBacked = true
			becameVisible = dbcRep.DefaultFlags&factionFlagVisible == 0 && !factionVisibleGated(dbcRep.DefaultFlags)
			total := dbcRep.BaseStanding + amount
			if total > 42999 {
				total = 42999
			} else if total < -42000 {
				total = -42000
			}
			rep = playerReputation{
				FactionID: factionID,
				ListID:    uint32(dbcRep.ReputationList),
				Base:      dbcRep.BaseStanding,
				Standing:  total - dbcRep.BaseStanding,
				Flags:     dbcRep.DefaultFlags,
			}
			if rep.Flags&(factionFlagInvisibleForced|factionFlagHidden) == 0 || rep.Flags&factionFlagSpecial != 0 {
				rep.Flags |= factionFlagVisible
			}
			if reputationAutoWar(rep.Flags, total, rep.Standing) {
				rep.Flags |= factionFlagAtWar
			}
			oldRank = reputationRank(int64(dbcRep.BaseStanding))
		}
	}
	if !dbcBacked {
		if rep.Standing > 42999 {
			rep.Standing = 42999
		} else if rep.Standing < -42000 {
			rep.Standing = -42000
		}
	}
	s.player.Reputations = append(s.player.Reputations, rep)
	_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_reputation (guid, faction, standing, flags) VALUES (?, ?, ?, ?)", s.playerGUID, factionID, rep.Standing, rep.Flags)
	if becameVisible {
		s.sendFactionVisiblePacket(rep.ListID)
	}
	// ReputationMgr::SetReputation (ReputationMgr.cpp:369): new factions are
	// reported immediately as well ("only this faction gets reported to
	// client, even if it has no own visible standing").
	_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState([]playerReputation{rep}, reputationRank(int64(totalReputationStanding(rep))) > oldRank), true)
	s.fireReputationCriteria(factionID, totalReputationStanding(rep))
	s.questReputationChanged(ctx, factionID)
}

// setReputationStanding mirrors the non-incremental leg of
// ReputationMgr::SetReputation (ReputationMgr.cpp:300-372) as reached from
// Player:SetReputation (PlayerMethods.h:1778-1785): the Eluna hook fires
// first with incremental=false and a rewritten standing becomes the new
// absolute target (no RATE_REPUTATION_GAIN scaling — that fold lives only
// in the incremental SetOneFactionReputation arm). Spillover runs first
// via applyReputationSpillover, then the main-faction write mirrors
// SetOneFactionReputation's non-incremental arm: clamp the total to
// [-42000, 42999], store net of the base, SetVisible fold, hostile auto-war
// arm, immediate SendState, criteria, and the ReputationChanged quest
// scan. A faction with no recorded row and no DBC row is a no-op, the
// SetOneFactionReputation false arm (C++ would deref a nil FactionEntry).
func (s *session) setReputationStanding(ctx context.Context, factionID uint32, target int32) {
	if s == nil || s.player == nil || factionID == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	target = s.fireReputationChangeHook(ctx, factionID, target, false)
	// ReputationMgr::SetReputation (ReputationMgr.cpp:302-358): spillover
	// runs before the main-faction write, on the hook-rewritten target.
	s.applyReputationSpillover(ctx, factionID, target, false)
	cdb := s.server.CharactersStore.DB
	for i := range s.player.Reputations {
		if s.player.Reputations[i].FactionID == factionID {
			oldRank := reputationRank(int64(totalReputationStanding(s.player.Reputations[i])))
			newTotal := target
			if newTotal > 42999 {
				newTotal = 42999
			} else if newTotal < -42000 {
				newTotal = -42000
			}
			s.player.Reputations[i].Standing = newTotal - s.player.Reputations[i].Base
			becameVisible := false
			if !factionVisibleGated(s.player.Reputations[i].Flags) && s.player.Reputations[i].Flags&factionFlagVisible == 0 {
				s.player.Reputations[i].Flags |= factionFlagVisible
				becameVisible = true
			}
			if reputationAutoWar(s.player.Reputations[i].Flags, newTotal, s.player.Reputations[i].Standing) {
				s.player.Reputations[i].Flags |= factionFlagAtWar
			}
			_, _ = cdb.ExecContext(ctx, "UPDATE character_reputation SET standing = ?, flags = ? WHERE guid = ? AND faction = ?", s.player.Reputations[i].Standing, s.player.Reputations[i].Flags, s.playerGUID, factionID)
			if becameVisible {
				s.sendFactionVisiblePacket(s.player.Reputations[i].ListID)
			}
			_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState([]playerReputation{s.player.Reputations[i]}, reputationRank(int64(newTotal)) > oldRank), true)
			s.fireReputationCriteria(factionID, newTotal)
			s.questReputationChanged(ctx, factionID)
			return
		}
	}
	if s.server.Data == nil {
		return
	}
	dbcRep, found, dbcErr := s.server.Data.Reputation(factionID, s.player.Race, s.player.Class)
	if dbcErr != nil || !found || dbcRep.ReputationList < 0 {
		return
	}
	newTotal := target
	if newTotal > 42999 {
		newTotal = 42999
	} else if newTotal < -42000 {
		newTotal = -42000
	}
	rep := playerReputation{
		FactionID: factionID,
		ListID:    uint32(dbcRep.ReputationList),
		Base:      dbcRep.BaseStanding,
		Standing:  newTotal - dbcRep.BaseStanding,
		Flags:     dbcRep.DefaultFlags,
	}
	if rep.Flags&(factionFlagInvisibleForced|factionFlagHidden) == 0 || rep.Flags&factionFlagSpecial != 0 {
		rep.Flags |= factionFlagVisible
	}
	if reputationAutoWar(rep.Flags, newTotal, rep.Standing) {
		rep.Flags |= factionFlagAtWar
	}
	becameVisible := dbcRep.DefaultFlags&factionFlagVisible == 0 && !factionVisibleGated(dbcRep.DefaultFlags)
	s.player.Reputations = append(s.player.Reputations, rep)
	_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_reputation (guid, faction, standing, flags) VALUES (?, ?, ?, ?)", s.playerGUID, factionID, rep.Standing, rep.Flags)
	if becameVisible {
		s.sendFactionVisiblePacket(rep.ListID)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState([]playerReputation{rep}, reputationRank(int64(newTotal)) > reputationRank(int64(dbcRep.BaseStanding))), true)
	s.fireReputationCriteria(factionID, newTotal)
	s.questReputationChanged(ctx, factionID)
}

// repSpilloverTemplate is one row of the world `reputation_spillover_template`
// table (ObjectMgr::LoadReputationSpilloverTemplate, ObjectMgr.cpp:8053):
// up to 4 spillover factions, each with a rate and a rank cap.
type repSpilloverTemplate struct {
	faction [4]uint32
	rate    [4]float32
	rank    [4]uint32
}

// loadRepSpilloverTemplate mirrors the C++ loader's validation
// (ObjectMgr.cpp:8074-8136): the main faction must exist in Faction.dbc
// with a parent faction, and every non-zero spillover faction must exist
// with a reputation list and a rank below 8; any invalid slot drops the
// whole template, and a missing row means no template at all. The table is
// read per call, so `.reload reputation_spillover_template` takes effect
// on the next reputation change without a cache to invalidate.
func (s *session) loadRepSpilloverTemplate(ctx context.Context, factionID uint32) (*repSpilloverTemplate, bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return nil, false
	}
	parent, found, err := s.server.Data.FactionParent(factionID)
	if err != nil || !found || parent.ParentID == 0 {
		return nil, false
	}
	var faction, rank [4]int64
	var rate [4]float64
	err = s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT faction1, rate_1, rank_1, faction2, rate_2, rank_2, faction3, rate_3, rank_3, faction4, rate_4, rank_4 FROM reputation_spillover_template WHERE faction = ?`, factionID).Scan(
		&faction[0], &rate[0], &rank[0], &faction[1], &rate[1], &rank[1], &faction[2], &rate[2], &rank[2], &faction[3], &rate[3], &rank[3])
	if err != nil {
		return nil, false
	}
	template := &repSpilloverTemplate{}
	for i := 0; i < 4; i++ {
		if faction[i] == 0 {
			continue
		}
		rep, found, repErr := s.server.Data.Reputation(uint32(faction[i]), s.player.Race, s.player.Class)
		if repErr != nil || !found || rep.ReputationList < 0 || rank[i] >= 8 {
			return nil, false
		}
		template.faction[i] = uint32(faction[i])
		template.rate[i] = float32(rate[i])
		template.rank[i] = uint32(rank[i])
	}
	return template, true
}

// repSource names the Player::CalculateReputationGain source arms
// (Player.cpp:6651-6726); the reward-rate row picks its column by source.
type repSource int

const (
	repSourceKill repSource = iota
	repSourceQuest
	repSourceDailyQuest
	repSourceWeeklyQuest
	repSourceMonthlyQuest
	repSourceRepeatableQuest
	repSourceSpell
)

// repRewardRate is one row of the world `reputation_reward_rate` table
// (ObjectMgr::LoadReputationRewardRate, ObjectMgr.cpp:7893; RepRewardRate,
// ObjectMgr.h:757).
type repRewardRate struct {
	quest           float32
	questDaily      float32
	questWeekly     float32
	questMonthly    float32
	questRepeatable float32
	creature        float32
	spell           float32
}

// rateFor returns the row's column for the source. C++ initializes the
// picked rate to 0.0 before the switch (Player.cpp:6685), so an unknown
// source reads 0 and the caller returns 0, matching the C++ fallthrough.
func (r *repRewardRate) rateFor(source repSource) float32 {
	switch source {
	case repSourceKill:
		return r.creature
	case repSourceQuest:
		return r.quest
	case repSourceDailyQuest:
		return r.questDaily
	case repSourceWeeklyQuest:
		return r.questWeekly
	case repSourceMonthlyQuest:
		return r.questMonthly
	case repSourceRepeatableQuest:
		return r.questRepeatable
	case repSourceSpell:
		return r.spell
	default:
		return 0
	}
}

// loadRepRewardRate mirrors the C++ loader's validation
// (ObjectMgr.cpp:7916-7971): the faction must exist in Faction.dbc and no
// rate may be negative; any failure drops the whole row, and a missing row
// means no reward rate at all. The table is read per call, so `.reload
// reputation_reward_rate` takes effect on the next reputation change
// without a cache to invalidate.
func (s *session) loadRepRewardRate(ctx context.Context, factionID uint32) (*repRewardRate, bool) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return nil, false
	}
	if !s.spellFactionEntryExists(factionID) {
		return nil, false
	}
	var rate repRewardRate
	err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT quest_rate, quest_daily_rate, quest_weekly_rate, quest_monthly_rate, quest_repeatable_rate, creature_rate, spell_rate FROM reputation_reward_rate WHERE faction = ?`, factionID).Scan(
		&rate.quest, &rate.questDaily, &rate.questWeekly, &rate.questMonthly, &rate.questRepeatable, &rate.creature, &rate.spell)
	if err != nil {
		return nil, false
	}
	if rate.quest < 0 || rate.questDaily < 0 || rate.questWeekly < 0 || rate.questMonthly < 0 || rate.questRepeatable < 0 || rate.creature < 0 || rate.spell < 0 {
		return nil, false
	}
	return &rate, true
}

// applyRepRewardRate folds a reward-rate row into percent in C++ order
// (Player.cpp:6688-6721): a zero rate disables the gain outright, any
// other rate multiplies percent. It reports false when the gain is
// disabled, so callers return 0 without further folds.
func (s *session) applyRepRewardRate(ctx context.Context, factionID uint32, source repSource, percent float32) (float32, bool) {
	rate, ok := s.loadRepRewardRate(ctx, factionID)
	if !ok {
		return percent, true
	}
	picked := rate.rateFor(source)
	if picked <= 0 {
		return 0, false
	}
	return percent * picked, true
}

// applySpilloverReputation mirrors ReputationMgr::SetOneFactionReputation
// (ReputationMgr.cpp:374) as called from SetReputation's spillover arms:
// no Eluna hook (the hook fires once for the main faction at SetReputation
// entry), no aura-percent fold, no RATE_REPUTATION_GAIN fold (Go has no
// rate model; the C++ default rate 1.0 makes the incremental fold
// identity), and no chained spillover. The existing-row arm mirrors the
// main sinks (clamp to [-42000, 42999], store net of the base, SetVisible
// fold, hostile auto-war, immediate SendState, criteria, quest scan); the
// no-row arm creates only a DBC-backed row, and a DBC miss is the SetOne
// false arm (no-op), not the incremental sink's phantom row.
func (s *session) applySpilloverReputation(ctx context.Context, factionID uint32, amount int32, incremental bool) {
	if s == nil || s.player == nil || factionID == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	for i := range s.player.Reputations {
		if s.player.Reputations[i].FactionID != factionID {
			continue
		}
		oldRank := reputationRank(int64(totalReputationStanding(s.player.Reputations[i])))
		newTotal := amount
		if incremental {
			newTotal = totalReputationStanding(s.player.Reputations[i]) + amount
		}
		if newTotal > 42999 {
			newTotal = 42999
		} else if newTotal < -42000 {
			newTotal = -42000
		}
		s.player.Reputations[i].Standing = newTotal - s.player.Reputations[i].Base
		becameVisible := false
		if !factionVisibleGated(s.player.Reputations[i].Flags) && s.player.Reputations[i].Flags&factionFlagVisible == 0 {
			s.player.Reputations[i].Flags |= factionFlagVisible
			becameVisible = true
		}
		if reputationAutoWar(s.player.Reputations[i].Flags, newTotal, s.player.Reputations[i].Standing) {
			s.player.Reputations[i].Flags |= factionFlagAtWar
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE character_reputation SET standing = ?, flags = ? WHERE guid = ? AND faction = ?", s.player.Reputations[i].Standing, s.player.Reputations[i].Flags, s.playerGUID, factionID)
		if becameVisible {
			s.sendFactionVisiblePacket(s.player.Reputations[i].ListID)
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState([]playerReputation{s.player.Reputations[i]}, reputationRank(int64(newTotal)) > oldRank), true)
		s.fireReputationCriteria(factionID, newTotal)
		s.questReputationChanged(ctx, factionID)
		return
	}
	if s.server.Data == nil {
		return
	}
	dbcRep, found, dbcErr := s.server.Data.Reputation(factionID, s.player.Race, s.player.Class)
	if dbcErr != nil || !found || dbcRep.ReputationList < 0 {
		return
	}
	newTotal := amount
	if incremental {
		newTotal = dbcRep.BaseStanding + amount
	}
	if newTotal > 42999 {
		newTotal = 42999
	} else if newTotal < -42000 {
		newTotal = -42000
	}
	rep := playerReputation{
		FactionID: factionID,
		ListID:    uint32(dbcRep.ReputationList),
		Base:      dbcRep.BaseStanding,
		Standing:  newTotal - dbcRep.BaseStanding,
		Flags:     dbcRep.DefaultFlags,
	}
	if rep.Flags&(factionFlagInvisibleForced|factionFlagHidden) == 0 || rep.Flags&factionFlagSpecial != 0 {
		rep.Flags |= factionFlagVisible
	}
	if reputationAutoWar(rep.Flags, newTotal, rep.Standing) {
		rep.Flags |= factionFlagAtWar
	}
	becameVisible := dbcRep.DefaultFlags&factionFlagVisible == 0 && !factionVisibleGated(dbcRep.DefaultFlags)
	s.player.Reputations = append(s.player.Reputations, rep)
	_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_reputation (guid, faction, standing, flags) VALUES (?, ?, ?, ?)", s.playerGUID, factionID, rep.Standing, rep.Flags)
	if becameVisible {
		s.sendFactionVisiblePacket(rep.ListID)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState([]playerReputation{rep}, reputationRank(int64(newTotal)) > reputationRank(int64(dbcRep.BaseStanding))), true)
	s.fireReputationCriteria(factionID, newTotal)
	s.questReputationChanged(ctx, factionID)
}

// applyReputationSpillover mirrors ReputationMgr::SetReputation's spillover
// legs (ReputationMgr.cpp:302-358), run before the main-faction write.
// standing is the hook-rewritten value: a delta when incremental, an
// absolute target otherwise. A DB template overrides the DBC parent model
// entirely: each slot spills int32(standing * rate) while the target's
// current rank is at or below the slot's rank cap. Otherwise the DBC arm
// spills to the faction's sub-factions (factions whose parent is this
// faction); with no sub-factions, a faction with a parent and a non-zero
// outgoing mod spills to the parent only when the parent's own row is
// flagged SPECIAL, and otherwise to the parent's other sub-factions
// ("sisters"). Each sister is skipped when its current rank exceeds its
// incoming cap, and a zero spill is skipped on the incremental path but
// still writes on the absolute path. Rank gates read pre-spillover totals,
// matching C++ GetRank on the untouched states.
func (s *session) applyReputationSpillover(ctx context.Context, factionID uint32, standing int32, incremental bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	if template, ok := s.loadRepSpilloverTemplate(ctx, factionID); ok {
		for i := 0; i < 4; i++ {
			if template.faction[i] == 0 {
				continue
			}
			if reputationRank(int64(s.reputationStanding(template.faction[i]))) <= template.rank[i] {
				s.applySpilloverReputation(ctx, template.faction[i], int32(float32(standing)*template.rate[i]), incremental)
			}
		}
		return
	}
	parent, found, err := s.server.Data.FactionParent(factionID)
	if err != nil || !found {
		return
	}
	spillOut := float32(standing)
	members, err := s.server.Data.FactionTeamMembers(factionID)
	if err != nil {
		return
	}
	if len(members) == 0 && parent.ParentID != 0 && parent.ModOut != 0 {
		spillOut *= parent.ModOut
		specialParent := false
		for i := range s.player.Reputations {
			if s.player.Reputations[i].FactionID == parent.ParentID && s.player.Reputations[i].Flags&factionFlagSpecial != 0 {
				specialParent = true
				break
			}
		}
		if specialParent {
			s.applySpilloverReputation(ctx, parent.ParentID, int32(spillOut), incremental)
			return
		}
		members, err = s.server.Data.FactionTeamMembers(parent.ParentID)
		if err != nil {
			return
		}
	}
	for _, member := range members {
		if member == factionID {
			continue
		}
		memberParent, found, err := s.server.Data.FactionParent(member)
		if err != nil || !found {
			continue
		}
		if reputationRank(int64(s.reputationStanding(member))) > memberParent.CapIn {
			continue
		}
		spill := int32(spillOut * memberParent.ModIn)
		if spill != 0 || !incremental {
			s.applySpilloverReputation(ctx, member, spill, incremental)
		}
	}
}

// spilloverOnlyReputation mirrors ModifyReputation's spillOverOnly arm
// (ReputationMgr.h:123, Player.cpp:6765): the hook fires once with the
// delta, then only the spillover legs run and the faction itself is never
// updated.
func (s *session) spilloverOnlyReputation(ctx context.Context, factionID uint32, gain int32) {
	if s == nil || s.player == nil {
		return
	}
	gain = s.fireReputationChangeHook(ctx, factionID, gain, true)
	s.applyReputationSpillover(ctx, factionID, gain, true)
}

// questReputationChanged mirrors Player::ReputationChanged and
// Player::ReputationChanged2 (Player.cpp:16893-16941): a standing change
// re-scans the quest log for quests with a matching rep objective
// (RepObjectiveFaction/Value and the Faction2 pair). An incomplete quest
// whose threshold is now met completes (gated by CanCompleteQuest); a
// complete quest that fell below its threshold flips back via
// IncompleteQuest.
func (s *session) questReputationChanged(ctx context.Context, factionID uint32) {
	if s == nil || s.player == nil || factionID == 0 || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	wdb := s.server.WorldStore.DB
	standing := int64(s.reputationStanding(factionID))
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		questID := s.player.QuestLog[slot].QuestID
		if questID == 0 {
			continue
		}
		var fac1, val1, fac2, val2 int64
		if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(RepObjectiveFaction, 0), COALESCE(RepObjectiveValue, 0), COALESCE(RepObjectiveFaction2, 0), COALESCE(RepObjectiveValue2, 0) FROM quest_template WHERE ID = ?", questID).Scan(&fac1, &val1, &fac2, &val2); err != nil {
			continue
		}
		threshold := int64(0)
		matched := false
		if fac1 == int64(factionID) {
			threshold, matched = val1, true
		} else if fac2 == int64(factionID) {
			threshold, matched = val2, true
		}
		if !matched {
			continue
		}
		status, err := s.characterQuestStatus(ctx, questID)
		if err != nil {
			continue
		}
		switch status {
		case questStatusIncomplete:
			if standing >= threshold && s.canCompleteQuest(ctx, questID) {
				s.completeQuest(ctx, questID)
			}
		case questStatusComplete:
			if standing < threshold {
				s.incompleteQuest(ctx, questID)
			}
		}
	}
}

// setFactionVisible mirrors ReputationMgr::SetVisible(FactionState*)
// (ReputationMgr.cpp:445-460): factions flagged INVISIBLE_FORCED or
// HIDDEN without SPECIAL can never be made visible; an already-visible
// faction is a no-op; otherwise the VISIBLE flag is set, the row is
// saved, and SMSG_SET_FACTION_VISIBLE carries the faction's
// ReputationList id. C++ always has the faction row (ReputationMgr::
// Initialize), so a faction with no Go row is created with the
// C++-equivalent defaults (standing 0, race-masked default flags).
// Callers: the RepObjectiveFaction/2 arms of Player::AddQuest
// (Player.cpp:15241-15247) and the gossip-hello NPC faction arm
// (NPCHandler.cpp:163).
func (s *session) setFactionVisible(ctx context.Context, factionID uint32) {
	if s == nil || s.player == nil || factionID == 0 || s.server == nil || s.server.Data == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	// sFactionStore.LookupEntry gate: a faction with no Faction.dbc
	// entry, or a ReputationIndex < 0, has no visible state in C++.
	rep, found, err := s.server.Data.Reputation(factionID, s.player.Race, s.player.Class)
	if err != nil || !found || rep.ReputationList < 0 {
		return
	}
	visibleGated := func(flags uint8) bool {
		return flags&(factionFlagInvisibleForced|factionFlagHidden) != 0 && flags&factionFlagSpecial == 0
	}
	for i := range s.player.Reputations {
		if s.player.Reputations[i].FactionID != factionID {
			continue
		}
		if visibleGated(s.player.Reputations[i].Flags) {
			return
		}
		if s.player.Reputations[i].Flags&factionFlagVisible != 0 {
			return
		}
		s.player.Reputations[i].Flags |= factionFlagVisible
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_reputation SET flags = ? WHERE guid = ? AND faction = ?", s.player.Reputations[i].Flags, s.playerGUID, factionID)
		packet := protocol.NewBuffer(4)
		packet.WriteU32(s.player.Reputations[i].ListID)
		_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_VISIBLE), packet.Bytes(), true)
		return
	}
	newRep := playerReputation{FactionID: factionID, ListID: uint32(rep.ReputationList), Standing: 0, Base: rep.BaseStanding, Flags: rep.DefaultFlags}
	if visibleGated(newRep.Flags) {
		return
	}
	newRep.Flags |= factionFlagVisible
	s.player.Reputations = append(s.player.Reputations, newRep)
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_reputation (guid, faction, standing, flags) VALUES (?, ?, ?, ?)", s.playerGUID, factionID, newRep.Standing, newRep.Flags)
	packet := protocol.NewBuffer(4)
	packet.WriteU32(newRep.ListID)
	_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_VISIBLE), packet.Bytes(), true)
}

// spellFactionEntryExists mirrors the sFactionStore.LookupEntry gate in
// Spell::EffectReputation (SpellEffects.cpp:4359-4361): a reputation spell
// whose MiscValue names no Faction DBC entry grants nothing. A DBC load
// failure degrades to the old credit-anyway behavior rather than silently
// dropping reputation.
func (s *session) spellFactionEntryExists(factionID uint32) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return true
	}
	file, err := s.server.Data.File("Faction")
	if err != nil || file == nil {
		return true
	}
	_, ok := file.Find(factionID)
	return ok
}

// reputationPriceDiscount mirrors Player::GetReputationPriceDiscount
// (Player.cpp:23674-23689): the vendor/repair price multiplier from the
// player's reputation rank with the NPC's faction template's faction — 1.0 at
// neutral and below, minus 5% per rank above neutral. The faction template id
// comes from creature_template (faction_A for Alliance, faction_H for Horde)
// resolved to its FactionTemplate.dbc faction; anything unresolvable keeps
// the 1.0 no-discount default.
func (s *session) reputationPriceDiscount(ctx context.Context, npcGUID uint64) float64 {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil ||
		s.server.WorldStore.DB == nil || s.server.Data == nil {
		return 1.0
	}
	entry := uint32((npcGUID >> 24) & 0x00FFFFFF)
	if entry == 0 {
		return 1.0
	}
	var factionA, factionH int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx,
		"SELECT faction_A, faction_H FROM creature_template WHERE entry = ? LIMIT 1", entry).Scan(&factionA, &factionH); err != nil {
		return 1.0
	}
	templateID := factionA
	if !s.playerAlliance() {
		templateID = factionH
	}
	if templateID <= 0 {
		return 1.0
	}
	return s.reputationDiscountForFactionTemplate(uint32(templateID))
}

// reputationDiscountForFactionTemplate mirrors
// Player::GetReputationPriceDiscount(FactionTemplateEntry const*)
// (Player.cpp:23679-23688): 1.0 at or below neutral, -5% per rank above.
// FlightPathMovementGenerator::LoadPath recomputes the discount from the
// stored flight-master faction template when a flight resumes after relog.
func (s *session) reputationDiscountForFactionTemplate(templateID uint32) float64 {
	const neutralRank = 3 // REP_NEUTRAL (SharedDefines.h:209-220)
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 1.0
	}
	ft, ok, err := s.server.Data.FactionTemplate(templateID)
	if err != nil || !ok || ft.Faction == 0 {
		return 1.0
	}
	var standing int64
	found := false
	for _, rep := range s.player.Reputations {
		if rep.FactionID == ft.Faction {
			standing = int64(totalReputationStanding(rep))
			found = true
			break
		}
	}
	if !found {
		return 1.0
	}
	rank := reputationRank(standing)
	if rank <= neutralRank {
		return 1.0
	}
	return 1.0 - 0.05*float64(rank-neutralRank)
}

// fireReputationChangeHook fires Eluna PLAYER_EVENT_ON_REPUTATION_CHANGE
// (15): Eluna::OnReputationChange (PlayerHooks.cpp:254) via
// ScriptMgr::OnPlayerReputationChange (ScriptMgr.cpp:2037). C++ pushes
// (player, factionId, standing, incremental) and lets each handler rewrite
// the standing through a numeric return — the chained-argument convention
// (ReplaceArgument), the same trigger as fireGiveXPHook. It returns the
// final standing the engine must apply.
func (s *session) fireReputationChangeHook(ctx context.Context, factionID uint32, standing int32, incremental bool) int32 {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return standing
	}
	args := []any{scripting.PlayerEventReputationChange, s.luaPlayer(), factionID, standing, incremental}
	_, err := s.server.Features.Scripts.TriggerPlayerEventUpdated(ctx, scripting.PlayerEventReputationChange, args, func(returns []any) {
		if len(returns) == 0 {
			return
		}
		if n, ok := returns[0].(float64); ok {
			standing = int32(n)
			args[3] = standing
		}
	})
	if err != nil {
		s.debug("lua reputation-change hook failed", "account", s.accountName, "error", err)
	}
	return standing
}

// onkillReputation is one row of the world `creature_onkill_reputation` table
// (ObjectMgr::LoadReputationOnKill, ObjectMgr.cpp:7981), the ReputationOnKillEntry
// behind Player::RewardReputation(Unit*, float) (Player.cpp:6731).
type onkillReputation struct {
	repFaction1   uint32
	repFaction2   uint32
	maxCap1       uint8
	maxCap2       uint8
	repValue1     int32
	repValue2     int32
	teamDependent bool
}

// loadOnkillReputation fetches the kill-reputation row for a creature entry.
// Columns are named explicitly: the sqlite schema orders MaxStanding before
// IsTeamAward, unlike the C++ SELECT position list, so positional reads would
// cross the streams.
func loadOnkillReputation(ctx context.Context, db *sql.DB, creatureEntry uint32) (*onkillReputation, bool) {
	if db == nil || creatureEntry == 0 {
		return nil, false
	}
	var rep onkillReputation
	var teamDependent int64
	err := db.QueryRowContext(ctx, `SELECT RewOnKillRepFaction1, RewOnKillRepFaction2,
		MaxStanding1, MaxStanding2, RewOnKillRepValue1, RewOnKillRepValue2, TeamDependent
		FROM creature_onkill_reputation WHERE creature_id = ?`, creatureEntry).Scan(
		&rep.repFaction1, &rep.repFaction2, &rep.maxCap1, &rep.maxCap2,
		&rep.repValue1, &rep.repValue2, &teamDependent)
	if err != nil {
		return nil, false
	}
	rep.teamDependent = teamDependent != 0
	return &rep, true
}

// championingSpellFactions maps the tabard championing dummy-aura spells to
// their factions (AuraEffect::HandleAuraDummy, SpellAuraEffects.cpp:4599:
// apply sets m_ChampioningFaction, remove clears it).
var championingSpellFactions = map[uint32]uint32{
	57819: 1106, // Argent Crusade
	57820: 1098, // Knights of the Ebon Blade
	57821: 1090, // Kirin Tor
	57822: 1091, // The Wyrmrest Accord
}

// championingFaction resolves Player::GetChampioningFaction for the kill-
// reputation arm (Player.cpp:6743-6754): one of the championing auras must be
// live on the player, the kill must happen in a non-raid dungeon, and the
// LFG dungeon entry for the map+difficulty must target level 80
// (GetLFGDungeon, DBCStores.cpp:910 — first match wins).
func (s *session) championingFaction(targetMapID uint32) uint32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var faction uint32
	for spellID, fac := range championingSpellFactions {
		if s.hasAura(spellID) {
			faction = fac
			break
		}
	}
	if faction == 0 {
		return 0
	}
	mapInfo, found, err := s.server.Data.Map(targetMapID)
	if err != nil || !found || !mapInfo.IsNonRaidDungeon() {
		return 0
	}
	dungeons, err := s.server.Data.LFGDungeons()
	if err != nil {
		return 0
	}
	difficulty := uint32(s.player.DungeonDifficulty)
	for _, d := range dungeons {
		if d.MapID == int32(targetMapID) && d.Difficulty == difficulty {
			if d.TargetLevel == 80 {
				return faction
			}
			return 0
		}
	}
	return 0
}

// calculateKillReputationGain mirrors Player::CalculateReputationGain
// (Player.cpp:6651-6726) for REPUTATION_SOURCE_KILL, in C++ order: the 156
// aura modifier folds into percent, plus the faction-specific 190 modifier
// which C++ gates to kills; the low-level kill rate (World.cpp:562,
// "Rate.Reputation.LowLevel.Kill", default 1.0) multiplies percent only
// when the victim's level is at or below the player's gray level; a zero
// or negative percent grants nothing; the creature_rate column folds next
// (a zero rate disables the gain); the recruit-a-friend bonus folds last
// for every source except SPELL (the kill path is SOURCE_KILL).
func (s *session) calculateKillReputationGain(ctx context.Context, factionID uint32, rep int32, mobLevel uint32) int32 {
	percent := float32(100)
	if mod := s.reputationGainPctMod(factionID, true); mod != 0 {
		if rep > 0 {
			percent += float32(mod)
		} else {
			percent -= float32(mod)
		}
	}
	killRate := float32(1)
	playerLevel := uint32(0)
	if s != nil {
		if s.player != nil {
			playerLevel = uint32(s.player.Level)
		}
		if s.server != nil {
			killRate = float32(s.server.Config.ReputationLowLevelKillRate)
		}
	}
	if killRate != 1 && mobLevel <= grayLevel(playerLevel) {
		percent *= killRate
	}
	if percent <= 0 {
		return 0
	}
	var ok bool
	if percent, ok = s.applyRepRewardRate(ctx, factionID, repSourceKill, percent); !ok {
		return 0
	}
	if s.getsRecruitAFriendBonus(false) {
		percent *= 1 + float32(s.server.Config.ReputationRecruitAFriendBonusRate)
	}
	return int32(float32(rep) * percent / 100)
}

// rewardKillReputation mirrors Player::RewardReputation(Unit* victim, float
// rate) (Player.cpp:6731-6779): each configured faction arm is gated on
// TeamDependent, scaled by CalculateReputationGain and the group rate
// (int32 truncation like C++'s int32(donerep * rate)), and skipped when the
// faction DBC row is missing. A rank past ReputationMaxCap flips
// ModifyReputation's spillOverOnly arm (Player.cpp:6765), which updates
// only spillover and never the faction itself. The uncapped sink is
// addReputationAmount, whose Eluna hook and persistence mirror
// ReputationMgr::SetReputation's main-faction arm.
func (s *session) rewardKillReputation(ctx context.Context, rep *onkillReputation, mobLevel uint32, rate float32, targetMapID uint32) {
	if s == nil || s.player == nil || s.server == nil || rep == nil {
		return
	}
	team := teamForRace(s.player.Race)
	championing := s.championingFaction(targetMapID)
	grant := func(repFaction uint32, repValue int32, maxCap uint8, teamOK bool) {
		if repFaction == 0 || !teamOK {
			return
		}
		faction := repFaction
		if championing != 0 {
			faction = championing
		}
		gain := s.calculateKillReputationGain(ctx, faction, repValue, mobLevel)
		gain = int32(float32(gain) * rate)
		if gain == 0 || !s.spellFactionEntryExists(faction) {
			return
		}
		if reputationRank(int64(s.reputationStanding(faction))) > uint32(maxCap) {
			s.spilloverOnlyReputation(ctx, faction, gain)
			return
		}
		s.addReputationAmount(ctx, faction, gain)
	}
	grant(rep.repFaction1, rep.repValue1, rep.maxCap1, !rep.teamDependent || team == 0)
	grant(rep.repFaction2, rep.repValue2, rep.maxCap2, !rep.teamDependent || team == 1)
}
