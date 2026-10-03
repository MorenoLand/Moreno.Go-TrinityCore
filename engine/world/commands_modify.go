package world

import (
	"context"
	"fmt"
	"strings"
)

// modify command port: modify_commandscript (cs_modify.cpp), the "morph" /
// "demorph" roots and the "modify" root with its 21 sub-arms plus the 6-arm
// "speed" sub-table. TWENTY-EIGHTH of 39 Commands groups
// (cs_script_loader.cpp decl 46 / call 91; call order re-verified this run:
// mmaps(90) -> modify(91)). Trinity checks permission only on the invoker
// leaf node (ChatCommand.cpp:487), so each arm gates exactly its own C++
// permission (RBAC.h:410-437/666); the root permission 544 covers the bare
// ".modify".
//
// This file is chunk 1: morph, demorph, and the modify arms hp, mana,
// energy, rage, runicpower, money, honor, arenapoints, xp and drunk.
// Chunk 2 ports the remaining arms: bit, faction, gender, mount, phase,
// reputation, scale, the speed sub-table (all/backwalk/fly/walk/swim/""),
// spell, standstate and talentpoints.
//
// Console-vs-chat branches are moot (Go commands are always sessioned).
// GetNameLink has no Go bridge, so the plain player name is used, per tree
// convention. LANG texts are inlined from the TDB enUS recall (no in-tree
// trinity_string seed).

// modifyTargetPlayer mirrors ChatHandler::getSelectedPlayerOrSelf
// (Chat.cpp:300): the selected online player, else the handler's own player.
// An unresolvable selection reports LANG_PLAYER_NOT_FOUND (499, "Player not
// found." per the tree convention).
func (s *session) modifyTargetPlayer(ctx context.Context) *session {
	_ = ctx
	target := s
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			target = ts
		} else {
			s.sendSysMessage("Player not found.")
			return nil
		}
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.")
		return nil
	}
	return target
}

// modifyTargetLowerSecurity mirrors the HasLowerSecurity guard shared by the
// modify arms: the command fails silently when the target's account outranks
// the handler's security level.
func (s *session) modifyTargetLowerSecurity(ctx context.Context, target *session) bool {
	if target != s && s.security < s.accountSecurityLevel(ctx, target.accountID) {
		return true // C++ HasLowerSecurity: silent fail
	}
	return false
}

// checkModifyResources mirrors modify_commandscript::CheckModifyResources
// (cs_modify.cpp:101): both values come from the same argument; either
// below 1 (or max below value) is LANG_BAD_VALUE (115, "Incorrect value."
// per the tree convention).
func (s *session) checkModifyResources(arg string) (res, resmax int32, ok bool) {
	v := cAtoi(arg)
	res, resmax = int32(v), int32(v)
	if res < 1 || resmax < 1 || resmax < res {
		s.sendSysMessage("Incorrect value.")
		return 0, 0, false
	}
	return res, resmax, true
}

// notifyModify mirrors modify_commandscript::NotifyModification
// (cs_modify.cpp:89): the handler gets the resource message and, when the
// target is a different player, the target gets the report message
// (needReportToTarget = target != handler; visibility is always true for
// online sessions here).
func (s *session) notifyModify(target *session, msg, targetMsg string) {
	s.sendSysMessage(msg)
	if target != s {
		target.sendSysMessage(targetMsg)
	}
}

// handleCmdMorph mirrors HandleModifyMorphCommand (cs_modify.cpp:811): the
// selected unit (online player only in the Go tree) or the handler's own
// player gets the display id via UNIT_FIELD_DISPLAYID.
func (s *session) handleCmdMorph(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .morph <displayid>")
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return
	}
	if s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	displayID := uint32(cAtoi(args[0]))
	target.player.TransformDisplayID = displayID
	target.sendPlayerUpdate()
}

// handleCmdDeMorph mirrors HandleDeMorphCommand (cs_modify.cpp:941): the
// selected unit (online player only in the Go tree) or the handler's own
// player drops the morph.
func (s *session) handleCmdDeMorph(ctx context.Context) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return
	}
	if s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	target.player.TransformDisplayID = 0
	target.sendPlayerUpdate()
}

// handleCmdModify dispatches the "modify" root (cs_modify.cpp:47-75). Chunk 1
// covers hp, mana, energy, rage, runicpower, money, honor, arenapoints, xp
// and drunk; the remaining arms land in chunk 2.
func (s *session) handleCmdModify(ctx context.Context, args []string) {
	const syntax = "Syntax: .modify hp|mana|energy|rage|runicpower|money|honor|arenapoints|xp|drunk|scale|spell|standstate|mount <val>"
	if len(args) == 0 {
		// Bare ".modify" matches the root node, whose own permission is 544.
		if s.miscDeny(ctx, permissionCommandModify) {
			return
		}
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("hp", sub):
		s.handleModifyHP(ctx, rest)
	case strings.HasPrefix("mana", sub):
		s.handleModifyPower(ctx, rest, 0, "mana", 1)
	case strings.HasPrefix("energy", sub):
		s.handleModifyPower(ctx, rest, 3, "energy", 10)
	case strings.HasPrefix("rage", sub):
		s.handleModifyPower(ctx, rest, 1, "rage", 10)
	case strings.HasPrefix("runicpower", sub):
		s.handleModifyPower(ctx, rest, 6, "runic power", 10)
	case strings.HasPrefix("money", sub):
		s.handleModifyMoney(ctx, rest)
	case strings.HasPrefix("honor", sub):
		s.handleModifyHonor(ctx, rest)
	case strings.HasPrefix("arenapoints", sub):
		s.handleModifyArena(ctx, rest)
	case strings.HasPrefix("xp", sub):
		s.handleModifyXP(ctx, rest)
	case strings.HasPrefix("drunk", sub):
		s.handleModifyDrunk(ctx, rest)
	default:
		if s.handleModifyChunk2a(ctx, sub, rest) {
			return
		}
		s.sendSysMessage(syntax)
	}
}

// handleModifyHP mirrors HandleModifyHPCommand (cs_modify.cpp:131).
func (s *session) handleModifyHP(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyHP) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify hp <val>")
		return
	}
	hp, hpmax, ok := s.checkModifyResources(args[0])
	if !ok {
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	// LANG_YOU_CHANGE_HP 118 / LANG_YOURS_HP_CHANGED 119.
	s.notifyModify(target,
		fmt.Sprintf("You change %s's HP to %d/%d.", target.player.Name, hp, hpmax),
		fmt.Sprintf("%s changed your HP to %d/%d.", s.player.Name, hp, hpmax))
	target.player.MaxHealth = uint32(hpmax)
	target.player.Health = uint32(hp)
	if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
		_, _ = chars.DB.ExecContext(ctx, "UPDATE characters SET health = ? WHERE guid = ?", target.player.Health, target.playerGUIDOf(target))
	}
	target.sendPlayerUpdate()
}

// handleModifyPower mirrors HandleModifyManaCommand / HandleModifyEnergyCommand /
// HandleModifyRageCommand / HandleModifyRunicPowerCommand (cs_modify.cpp:147-205):
// energy, rage and runic power take a x10 multiplier on the raw value.
func (s *session) handleModifyPower(ctx context.Context, args []string, powerIdx int, label string, multiplier int32) {
	var perm uint32
	switch powerIdx {
	case 0:
		perm = permissionCommandModifyMana
	case 1:
		perm = permissionCommandModifyRage
	case 3:
		perm = permissionCommandModifyEnergy
	default:
		perm = permissionCommandModifyRunicPower
	}
	if s.miscDeny(ctx, perm) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage(fmt.Sprintf("Syntax: .modify %s <val>", strings.ReplaceAll(label, " ", "")))
		return
	}
	v := cAtoi(args[0])
	res, resmax := int32(v)*multiplier, int32(v)*multiplier
	if res < 1 || resmax < 1 || resmax < res {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	shown, shownMax := res/multiplier, resmax/multiplier
	switch powerIdx {
	case 0: // LANG_YOU_CHANGE_MANA 120 / LANG_YOURS_MANA_CHANGED 121
		s.notifyModify(target,
			fmt.Sprintf("You change %s's mana to %d/%d.", target.player.Name, shown, shownMax),
			fmt.Sprintf("%s changed your mana to %d/%d.", s.player.Name, shown, shownMax))
	case 1: // LANG_YOU_CHANGE_RAGE 125 / LANG_YOURS_RAGE_CHANGED 126
		s.notifyModify(target,
			fmt.Sprintf("You change %s's rage to %d/%d.", target.player.Name, shown, shownMax),
			fmt.Sprintf("%s changed your rage to %d/%d.", s.player.Name, shown, shownMax))
	case 3: // LANG_YOU_CHANGE_ENERGY 122 / LANG_YOURS_ENERGY_CHANGED 123
		s.notifyModify(target,
			fmt.Sprintf("You change %s's energy to %d/%d.", target.player.Name, shown, shownMax),
			fmt.Sprintf("%s changed your energy to %d/%d.", s.player.Name, shown, shownMax))
	default: // LANG_YOU_CHANGE_RUNIC_POWER 173 / LANG_YOURS_RUNIC_POWER_CHANGED 174
		s.notifyModify(target,
			fmt.Sprintf("You change %s's runic power to %d/%d.", target.player.Name, shown, shownMax),
			fmt.Sprintf("%s changed your runic power to %d/%d.", s.player.Name, shown, shownMax))
	}
	target.player.MaxPowers[powerIdx] = uint32(resmax)
	target.player.Powers[powerIdx] = uint32(res)
	if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
		_, _ = chars.DB.ExecContext(ctx, fmt.Sprintf("UPDATE characters SET power%d = ? WHERE guid = ?", powerIdx+1), target.player.Powers[powerIdx], target.playerGUIDOf(target))
	}
	target.sendPlayerUpdate()
}

// parseModifyMoney mirrors the moneyToAddO leg of HandleModifyMoneyCommand
// (cs_modify.cpp:542-547): a value containing g/s/c is parsed by
// MoneyStringToMoney, otherwise it is a plain copper integer.
func parseModifyMoney(arg string) (int32, bool) {
	if strings.ContainsAny(arg, "gGsScC") {
		var total int64
		num := ""
		seen := false
		for _, r := range arg {
			switch {
			case r >= '0' && r <= '9':
				num += string(r)
			case r == 'g' || r == 'G':
				n, err := parseMoneyDigits(num)
				if err != nil {
					return 0, false
				}
				total += n * 10000
				num, seen = "", true
			case r == 's' || r == 'S':
				n, err := parseMoneyDigits(num)
				if err != nil {
					return 0, false
				}
				total += n * 100
				num, seen = "", true
			case r == 'c' || r == 'C':
				n, err := parseMoneyDigits(num)
				if err != nil {
					return 0, false
				}
				total += n
				num, seen = "", true
			default:
				return 0, false
			}
		}
		if !seen || num != "" {
			return 0, false
		}
		return int32(total), true
	}
	return int32(cAtoi(arg)), true
}

func parseMoneyDigits(num string) (int64, error) {
	if num == "" {
		return 0, fmt.Errorf("empty")
	}
	var n int64
	for _, r := range num {
		n = n*10 + int64(r-'0')
	}
	return n, nil
}

// handleModifyMoney mirrors HandleModifyMoneyCommand (cs_modify.cpp:535-605),
// including the take-all-money and MAX_MONEY_AMOUNT clamp legs.
func (s *session) handleModifyMoney(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyMoney) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify money <val>")
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	moneyToAdd, ok := parseModifyMoney(args[0])
	if !ok {
		return
	}
	targetMoney := target.player.Money
	if moneyToAdd < 0 {
		newmoney := int32(targetMoney) + moneyToAdd
		if newmoney <= 0 {
			// LANG_YOU_TAKE_ALL_MONEY 153 / LANG_YOURS_ALL_MONEY_GONE 154.
			s.notifyModify(target,
				fmt.Sprintf("You take all money from %s.", target.player.Name),
				fmt.Sprintf("%s takes all your money.", s.player.Name))
			target.player.Money = 0
		} else {
			if newmoney > int32(maxMoneyAmount) {
				newmoney = int32(maxMoneyAmount)
			}
			// LANG_YOU_TAKE_MONEY 155 / LANG_YOURS_MONEY_TAKEN 156.
			s.notifyModify(target,
				fmt.Sprintf("You take %d copper from %s.", -moneyToAdd, target.player.Name),
				fmt.Sprintf("%s takes %d of your copper.", s.player.Name, -moneyToAdd))
			target.player.Money = uint32(newmoney)
		}
	} else {
		// LANG_YOU_GIVE_MONEY 157 / LANG_YOURS_MONEY_GIVEN 158.
		s.notifyModify(target,
			fmt.Sprintf("You give %d copper to %s.", moneyToAdd, target.player.Name),
			fmt.Sprintf("%s gives you %d copper.", s.player.Name, moneyToAdd))
		if targetMoney >= maxMoneyAmount-uint32(moneyToAdd) {
			moneyToAdd -= int32(targetMoney) // C++ quirk, verbatim
		}
		newmoney := int64(targetMoney) + int64(moneyToAdd)
		if newmoney > int64(maxMoneyAmount) {
			newmoney = int64(maxMoneyAmount)
		}
		target.player.Money = uint32(newmoney)
	}
	if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
		_, _ = chars.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", target.player.Money, target.playerGUIDOf(target))
	}
	target.sendPlayerMoneyUpdate()
}

// handleModifyHonor mirrors HandleModifyHonorCommand (cs_modify.cpp:651).
func (s *session) handleModifyHonor(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyHonor) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify honor <val>")
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	amount := int32(cAtoi(args[0]))
	newValue := int64(target.player.TotalHonorPoints) + int64(amount)
	if newValue < 0 {
		newValue = 0
	}
	target.player.TotalHonorPoints = uint32(newValue) // Player::ModifyHonorPoints (Player.cpp:7069)
	target.persistHonorFields(ctx)
	target.sendPlayerUpdate()
	// LANG_COMMAND_MODIFY_HONOR 299.
	s.sendSysMessage(fmt.Sprintf("%s's honor points are now %d.", target.player.Name, target.player.TotalHonorPoints))
}

// handleModifyArena mirrors HandleModifyArenaCommand (cs_modify.cpp:878).
func (s *session) handleModifyArena(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyArenaPoints) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify arenapoints <val>")
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	amount := int32(cAtoi(args[0]))
	newValue := int64(target.player.ArenaPoints) + int64(amount)
	if newValue < 0 {
		newValue = 0
	}
	target.player.ArenaPoints = uint32(newValue) // Player::ModifyArenaPoints
	if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
		_, _ = chars.DB.ExecContext(ctx, "UPDATE characters SET arenaPoints = ? WHERE guid = ?", target.player.ArenaPoints, target.playerGUIDOf(target))
	}
	target.sendPlayerUpdate()
	// LANG_COMMAND_MODIFY_ARENA 306.
	s.sendSysMessage(fmt.Sprintf("%s's arena points are now %d.", target.player.Name, target.player.ArenaPoints))
}

// handleModifyXP mirrors HandleModifyXPCommand (cs_modify.cpp:966).
func (s *session) handleModifyXP(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyXP) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify xp <val>")
		return
	}
	xp := cAtoi(args[0])
	if xp < 1 {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	target.grantXP(ctx, uint32(xp)) // Player::GiveXP
}

// handleModifyDrunk mirrors HandleModifyDrunkCommand (cs_modify.cpp:673).
func (s *session) handleModifyDrunk(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyDrunk) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify drunk <val>")
		return
	}
	drunklevel := cAtoi(args[0])
	if drunklevel > 100 {
		drunklevel = 100
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return
	}
	// No security gate in C++ for the drunk arm; the drunken state lives in
	// PLAYER_BYTES_3, which is in-memory only (no characters column).
	target.player.DrunkenState = uint16(drunklevel)
	target.sendPlayerUpdate()
}

// playerGUIDOf returns the character guid of the given target session.
func (s *session) playerGUIDOf(target *session) uint64 {
	if target == s {
		return s.playerGUID
	}
	return target.playerGUID
}
