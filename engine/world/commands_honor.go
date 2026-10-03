package world

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// This file ports AddSC_honor_commandscript (cs_honor.cpp, whole 121-line file
// is 1 column-0 `class` def: 1 x `: public CommandScript` "honor_commandscript").
// Sole-source verified: the class is referenced only by cs_honor.cpp and
// cs_script_loader.cpp (decl 37 / call 82 - NINETEENTH of 39 groups; call
// order re-verified: guild(81) -> honor(82)). The Trinity "add" table's ""
// entry merges into the parent node (ChatCommand.cpp LoadCommandsIntoMap:53),
// so `honor add <amount>` invokes the add node directly while `honor add kill`
// descends to the kill child.

// honorAuraInactiveID is SPELL_AURA_PLAYER_INACTIVE (Battleground.h:139), the
// "Inactive" aura that blocks honor gains (Player::RewardHonor).
const honorAuraInactiveID = 43681

// spellAuraModHonorGainPct mirrors SPELL_AURA_MOD_HONOR_GAIN_PCT
// (SpellAuraDefines.h:361), the non-stacking honor-gain aura modifier.
const spellAuraModHonorGainPct = 281

// spellAuraNoPVPCredit mirrors SPELL_AURA_NO_PVP_CREDIT
// (SpellAuraDefines.h:239).
const spellAuraNoPVPCredit = 159

// handleCmdHonor mirrors the honor command table (cs_honor.cpp:46-59):
//
//	honor add <amount>  -> HandleHonorAddCommand   (RBAC_PERM_COMMAND_HONOR_ADD, 409)
//	honor add kill      -> HandleHonorAddKillCommand (RBAC_PERM_COMMAND_HONOR_ADD_KILL, 410)
//	honor update        -> HandleHonorUpdateCommand (RBAC_PERM_COMMAND_HONOR_UPDATE, 411)
//
// Trinity checks permission only on the invoker (leaf) node
// (ChatCommand.cpp:487), so each arm gates exactly its own C++ permission.
func (s *session) handleCmdHonor(ctx context.Context, args []string) {
	const syntax = "Syntax: .honor add <amount> | .honor add kill | .honor update"
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
	case strings.HasPrefix("add", sub):
		// The Trinity parser prefix-matches command names at every nesting
		// level; the "kill" child wins over the "" (bare-amount) arm exactly
		// like the C++ table order kill-then-"" (cs_honor.cpp:48-51).
		if len(args) > 1 && strings.HasPrefix("kill", strings.ToLower(args[1])) {
			if deny(permissionCommandHonorAddKill) {
				return
			}
			s.handleHonorAddKill(ctx)
			return
		}
		if deny(permissionCommandHonorAdd) {
			return
		}
		s.handleHonorAdd(ctx, args[1:])
	case strings.HasPrefix("update", sub):
		if deny(permissionCommandHonorUpdate) {
			return
		}
		s.handleHonorUpdate(ctx)
	default:
		s.sendSysMessage(syntax)
	}
}

// handleHonorAdd mirrors HandleHonorAddCommand (cs_honor.cpp:62): the target
// is the selected online player, else the handler's own player
// (ChatHandler::getSelectedPlayer falls back to self, Chat.cpp:300); an
// unresolvable selection reports LANG_PLAYER_NOT_FOUND (499, "Player not
// found." per the tree convention). HasLowerSecurity fails silently.
func (s *session) handleHonorAdd(ctx context.Context, args []string) {
	if len(args) != 1 {
		s.sendSysMessage("Syntax: .honor add <amount>")
		return
	}
	amount, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Syntax: .honor add <amount>")
		return
	}
	target := s
	if s.selection != 0 && s.server != nil {
		ts := s.server.playerSessionForGUID(s.selection)
		if ts == nil || ts.player == nil {
			s.sendSysMessage("Player not found.")
			return
		}
		target = ts
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.")
		return
	}
	if target != s && s.security < s.accountSecurityLevel(ctx, target.accountID) {
		return // C++ HasLowerSecurity: silent fail
	}
	target.rewardHonorPoints(ctx, uint32(amount))
}

// handleHonorAddKill mirrors HandleHonorAddKillCommand (cs_honor.cpp:79):
// getSelectedUnit returns the selected unit, else the handler's own player
// (Chat.cpp:312, never null for a sessioned handler), and the handler's own
// player earns the kill credit. The creature-victim path has no Go bridge:
// command selections resolve only to online player sessions, so a
// non-player selection reports the gap honestly instead of stubbing.
func (s *session) handleHonorAddKill(ctx context.Context) {
	if s.selection == 0 {
		return // victim == self: C++ RewardHonor returns false silently
	}
	var victim *session
	if s.server != nil {
		victim = s.server.playerSessionForGUID(s.selection)
	}
	if victim == nil || victim.player == nil {
		s.sendSysMessage("honor add kill on a creature is not supported: selected-unit creature targets have no Go bridge.")
		return
	}
	if s.security < s.accountSecurityLevel(ctx, victim.accountID) {
		return // C++ HasLowerSecurity: silent fail
	}
	s.rewardHonorKill(ctx, victim)
}

// handleHonorUpdate mirrors HandleHonorUpdateCommand (cs_honor.cpp:98).
func (s *session) handleHonorUpdate(ctx context.Context) {
	target := s
	if s.selection != 0 && s.server != nil {
		ts := s.server.playerSessionForGUID(s.selection)
		if ts == nil || ts.player == nil {
			s.sendSysMessage("Player not found.")
			return
		}
		target = ts
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.")
		return
	}
	if target != s && s.security < s.accountSecurityLevel(ctx, target.accountID) {
		return // C++ HasLowerSecurity: silent fail
	}
	target.updateHonorFields()
	target.persistHonorFields(ctx)
	target.sendPlayerUpdate()
}

// updateHonorFields mirrors Player::UpdateHonorFields (Player.cpp:6830):
// when the calendar day has rolled over since the last update, today's
// contribution and kill counts move into yesterday's fields (or are zeroed
// when the last update is older than yesterday).
func (s *session) updateHonorFields() {
	if s.player == nil {
		return
	}
	now := time.Now().Unix()
	today := now / 86400 * 86400
	if s.player.LastHonorUpdateTime == 0 {
		s.player.LastHonorUpdateTime = now
	}
	if s.player.LastHonorUpdateTime < today {
		if s.player.LastHonorUpdateTime >= today-86400 {
			s.player.YesterdayHonorPoints = s.player.TodayHonorPoints
			s.player.TodayHonorPoints = 0
			s.player.YesterdayKills = s.player.TodayKills
			s.player.TodayKills = 0
		} else {
			s.player.YesterdayHonorPoints = 0
			s.player.TodayKills = 0
			s.player.YesterdayKills = 0
		}
	}
	s.player.LastHonorUpdateTime = now
}

// rewardHonorPoints mirrors the victim==nullptr, explicit-honor path of
// Player::RewardHonor (Player.cpp:6865) used by HandleHonorAddCommand:
// RewardHonor(nullptr, 1, amount). The arena guards are moot: the Go tree has
// no arena model, so a session can never be in one.
func (t *session) rewardHonorPoints(ctx context.Context, amount uint32) {
	if t.player == nil {
		return
	}
	if t.hasAura(honorAuraInactiveID) {
		return
	}
	t.updateHonorFields()
	honor := int32(float64(amount) * t.honorRate())
	t.grantHonor(ctx, honor, 0, 0)
}

// rewardHonorKill mirrors the victim path of Player::RewardHonor
// (Player.cpp:6865) used by HandleHonorAddKillCommand:
// GetSession()->GetPlayer()->RewardHonor(target, 1). Only the player-victim
// branch is ported; creature victims have no command-target bridge (see
// handleHonorAddKill). The arena guards are moot (no Go arena model).
func (s *session) rewardHonorKill(ctx context.Context, victim *session) {
	if s.player == nil || victim == nil || victim.player == nil || victim == s {
		return
	}
	if s.hasAura(honorAuraInactiveID) {
		return
	}
	s.updateHonorFields()
	if victim.hasAuraType(spellAuraNoPVPCredit) {
		return
	}
	if teamForRace(s.player.Race) == teamForRace(victim.player.Race) && s.server.Config.GameType != 4 {
		return // same team, not an FFA-PvP realm (World::IsFFAPvPRealm = GameType 4)
	}
	killerLevel := s.player.Level
	killerGrey := uint8(grayLevel(uint32(killerLevel)))
	victimLevel := victim.player.Level
	if victimLevel <= killerGrey {
		return
	}
	// Victim title -> HK rank message (Player.cpp:6921-6934).
	var victimGUID uint64
	var victimRank uint32
	switch title := victim.player.ChosenTitle; {
	case title == 0:
		victimGUID = 0
	case title < 15:
		victimGUID = victim.playerGUID
		victimRank = title + 4
	case title < 29:
		victimGUID = victim.playerGUID
		victimRank = title - 14 + 4
	default:
		victimGUID = 0
	}
	honorF := math.Ceil(1.55 * float64(killerLevel) * float64(victimLevel-killerGrey) / float64(killerLevel-killerGrey))
	if s.player.TodayKills < ^uint16(0) {
		s.player.TodayKills++
	}
	s.player.TotalKills++
	// UpdateAchievementCriteria chain (Player.cpp:6940-6945).
	s.updateAchievementCriteria(criteriaTypeHonorableKill, 0, 1)
	s.updateAchievementCriteria(criteriaTypeEarnHonorableKill, 0, 1)
	if s.player.Zone > 0 {
		s.updateAchievementCriteria(criteriaTypeHKAtArea, s.player.Zone, 1)
	}
	s.updateAchievementCriteria(criteriaTypeHKClass, uint32(victim.player.Class), 1)
	s.updateAchievementCriteria(criteriaTypeHKRace, uint32(victim.player.Race), 1)
	s.updateAchievementCriteria(criteriaTypeSpecialPvPKill, 0, 1)
	// Non-stacking honor-gain aura modifier (Player.cpp:6983) then the world
	// honor rate (Player.cpp:6986).
	honorF *= 1 + float64(s.maxPositiveAuraModifier(spellAuraModHonorGainPct))/100
	honorF *= s.honorRate()
	s.grantHonor(ctx, int32(honorF), victimGUID, victimRank)
}

// grantHonor is the shared tail of Player::RewardHonor (Player.cpp:6988-7022):
// the SMSG_PVP_CREDIT packet, ModifyHonorPoints, today's contribution, and
// the battleground score update. CONFIG_PVP_TOKEN_ENABLE is false in the Go
// tree's config, so the token branch is moot; the battleground score bridge
// (Battleground::UpdatePlayerScore SCORE_BONUS_HONOR) is unbuilt and noted.
func (t *session) grantHonor(ctx context.Context, honor int32, victimGUID uint64, victimRank uint32) {
	t.sendPVPCredit(honor, victimGUID, victimRank)
	newValue := int64(t.player.TotalHonorPoints) + int64(honor)
	if newValue < 0 {
		newValue = 0
	}
	t.player.TotalHonorPoints = uint32(newValue) // Player::ModifyHonorPoints (Player.cpp:7069)
	if honor > 0 {
		if t.player.TotalHonorPoints > 0 {
			t.addKnownCurrency(t.player, itemHonorPointsID)
		}
		t.player.TodayHonorPoints += uint32(honor) // PLAYER_FIELD_TODAY_CONTRIBUTION
	}
	t.persistHonorFields(ctx)
	t.sendPlayerUpdate()
}

// sendPVPCredit writes SMSG_PVP_CREDIT (Player.cpp:6993): uint32 honor,
// uint64 victim guid, uint32 victim rank.
func (s *session) sendPVPCredit(honor int32, victimGUID uint64, victimRank uint32) {
	packet := protocol.NewBuffer(16)
	packet.WriteU32(uint32(honor))
	packet.WriteU64(victimGUID)
	packet.WriteU32(victimRank)
	_ = s.write(uint16(protocol.OpcodeSMSG_PVP_CREDIT), packet.Bytes(), true)
}

// persistHonorFields writes the live honor/kill fields back to the
// characters row.
func (s *session) persistHonorFields(ctx context.Context) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.player == nil {
		return
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
		"UPDATE characters SET totalHonorPoints = ?, todayHonorPoints = ?, yesterdayHonorPoints = ?, totalKills = ?, todayKills = ?, yesterdayKills = ? WHERE guid = ?",
		s.player.TotalHonorPoints, s.player.TodayHonorPoints, s.player.YesterdayHonorPoints,
		s.player.TotalKills, s.player.TodayKills, s.player.YesterdayKills, s.playerGUID)
}

// honorRate mirrors sWorld->getRate(RATE_HONOR) (worldserver.conf Rate.Honor).
func (s *session) honorRate() float64 {
	if s.server == nil {
		return 1
	}
	return s.server.Config.RateHonor
}
