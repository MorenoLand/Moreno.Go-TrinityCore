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
// (Player.cpp:6653-6659): the live 156 modifier plus the faction-specific
// 190 modifier whose effect MiscValue names this faction (the kill-only
// source gate has no Go model — all giveReputation callers share the sink).
// C++ applies it as percent = 100 + repMod for gains, 100 - repMod for
// losses (CalculatePct at the end). The quest-bonus, low-level rate and
// RepRewardRate legs have no Go model.
func (s *session) reputationGainPctMod(factionID uint32) (mod int32) {
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
				if uint32(effect.MiscValue) == factionID {
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
		s.debug("faction war state changed", "account", s.accountName, "faction", reputation.FactionID, "at_war", flag != 0)
		return true
	}
	return true
}

func (s *session) giveReputation(ctx context.Context, factionID uint32, amount int32) {
	if s.player == nil || factionID == 0 || amount == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	// Player::CalculateReputationGain (Player.cpp:6657): percent += rep > 0 ?
	// repMod : -repMod, then CalculatePct(rep, percent); a zero or negative
	// percent grants nothing.
	if mod := s.reputationGainPctMod(factionID); mod != 0 {
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
	if amount == 0 {
		return
	}
	cdb := s.server.CharactersStore.DB
	for i := range s.player.Reputations {
		if s.player.Reputations[i].FactionID == factionID {
			s.player.Reputations[i].Standing += amount
			_, _ = cdb.ExecContext(ctx, "UPDATE character_reputation SET standing = ? WHERE guid = ? AND faction = ?", s.player.Reputations[i].Standing, s.playerGUID, factionID)
			totalStanding := totalReputationStanding(s.player.Reputations[i])
			if totalStanding > 0 {
				s.setAchievementCriteria(criteriaTypeGainReputation, factionID, uint32(totalStanding))
				if totalStanding >= 9000 {
					s.updateAchievementCriteria(criteriaTypeHonoredRep, factionID, 1)
				}
				if totalStanding >= 21000 {
					s.updateAchievementCriteria(criteriaTypeReveredRep, factionID, 1)
				}
				// Reference GAIN_EXALTED_REPUTATION: any faction at 42000+ counts.
				if totalStanding >= 42000 {
					s.updateAchievementCriteria(criteriaTypeExaltedRep, factionID, 1)
				}
			}
			// Player::SetReputation (ReputationMgr.cpp:302) runs the quest
			// ReputationChanged scan after the standing lands.
			s.questReputationChanged(ctx, factionID)
			return
		}
	}
	rep := playerReputation{
		FactionID: factionID,
		Standing:  amount,
		Flags:     factionFlagVisible,
	}
	s.player.Reputations = append(s.player.Reputations, rep)
	_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_reputation (guid, faction, standing, flags) VALUES (?, ?, ?, ?)", s.playerGUID, factionID, amount, factionFlagVisible)
	s.questReputationChanged(ctx, factionID)
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
	const neutralRank = 3 // REP_NEUTRAL (SharedDefines.h:209-220)
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
	ft, ok, err := s.server.Data.FactionTemplate(uint32(templateID))
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
// (Player.cpp:6651-6726) for REPUTATION_SOURCE_KILL: the 156 aura modifier
// folds into percent, plus the faction-specific 190 modifier which C++ gates
// to kills (both already in reputationGainPctMod). The low-level kill rate
// (World.cpp:562, "Rate.Reputation.LowLevel.Kill", default 1.0) has no Go
// config, so the gray-level reduction arm is a no-op like the quest path.
// Player.cpp:6724: the recruit-a-friend bonus folds percent for every source
// except REPUTATION_SOURCE_SPELL (the kill path is SOURCE_KILL).
func (s *session) calculateKillReputationGain(factionID uint32, rep int32) int32 {
	percent := float32(100)
	if mod := s.reputationGainPctMod(factionID); mod != 0 {
		if rep > 0 {
			percent += float32(mod)
		} else {
			percent -= float32(mod)
		}
	}
	if s.getsRecruitAFriendBonus(false) {
		percent *= 1 + float32(s.server.Config.ReputationRecruitAFriendBonusRate)
	}
	if percent <= 0 {
		return 0
	}
	return int32(float32(rep) * percent / 100)
}

// rewardKillReputation mirrors Player::RewardReputation(Unit* victim, float
// rate) (Player.cpp:6731-6779): each configured faction arm is gated on
// TeamDependent, scaled by CalculateReputationGain and the group rate
// (int32 truncation like C++'s int32(donerep * rate)), and skipped when the
// faction DBC row is missing. A rank past ReputationMaxCap flips
// ModifyReputation's limit arm, which updates only spillover and never the
// faction itself — Go has no spillover model, so the grant is skipped. The
// sink is addReputationAmount, whose Eluna hook and persistence mirror
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
		gain := s.calculateKillReputationGain(faction, repValue)
		gain = int32(float32(gain) * rate)
		if gain == 0 || !s.spellFactionEntryExists(faction) {
			return
		}
		if reputationRank(int64(s.reputationStanding(faction))) > uint32(maxCap) {
			return
		}
		s.addReputationAmount(ctx, faction, gain)
	}
	grant(rep.repFaction1, rep.repValue1, rep.maxCap1, !rep.teamDependent || team == 0)
	grant(rep.repFaction2, rep.repValue2, rep.maxCap2, !rep.teamDependent || team == 1)
}
