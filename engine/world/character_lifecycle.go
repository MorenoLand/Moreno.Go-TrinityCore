package world

import (
	"context"
	"database/sql"
)

type characterGUIDReference struct{ table, column string }

var characterGUIDReferences = []characterGUIDReference{
	{"characters", "guid"}, {"character_account_data", "guid"}, {"character_achievement", "guid"}, {"character_achievement_progress", "guid"}, {"character_action", "guid"}, {"character_arena_stats", "guid"}, {"character_aura", "guid"}, {"character_banned", "guid"}, {"character_battleground_data", "guid"}, {"character_battleground_random", "guid"}, {"character_declinedname", "guid"}, {"character_equipmentsets", "guid"}, {"character_fishingsteps", "guid"}, {"character_gifts", "guid"}, {"character_glyphs", "guid"}, {"character_homebind", "guid"}, {"character_instance", "guid"}, {"character_inventory", "guid"}, {"character_queststatus", "guid"}, {"character_queststatus_daily", "guid"}, {"character_queststatus_monthly", "guid"}, {"character_queststatus_rewarded", "guid"}, {"character_queststatus_seasonal", "guid"}, {"character_queststatus_weekly", "guid"}, {"character_reputation", "guid"}, {"character_skills", "guid"}, {"character_social", "guid"}, {"character_spell", "guid"}, {"character_spell_cooldown", "guid"}, {"character_stats", "guid"}, {"character_talent", "guid"},
	{"character_pet", "owner"}, {"character_pet_declinedname", "owner"}, {"characters_npcbot", "owner"}, {"character_social", "friend"}, {"arena_team", "captainGuid"}, {"arena_team_member", "guid"}, {"auctionbidders", "bidderguid"}, {"auctionhouse", "itemowner"}, {"auctionhouse", "buyguid"}, {"battleground_deserters", "guid"}, {"calendar_invites", "invitee"}, {"calendar_invites", "sender"}, {"corpse", "guid"}, {"custom_transmogrification", "Owner"}, {"gm_survey", "guid"}, {"gm_ticket", "playerGuid"}, {"group_member", "memberGuid"}, {"groups", "leaderGuid"}, {"groups", "looterGuid"}, {"groups", "masterLooterGuid"}, {"guild", "leaderguid"}, {"guild_bank_eventlog", "PlayerGuid"}, {"guild_eventlog", "PlayerGuid1"}, {"guild_eventlog", "PlayerGuid2"}, {"guild_member", "guid"}, {"guild_member_withdraw", "guid"}, {"item_instance", "owner_guid"}, {"item_instance", "creatorGuid"}, {"item_instance", "giftCreatorGuid"}, {"item_refund_instance", "player_guid"}, {"lag_reports", "guid"}, {"lfg_data", "guid"}, {"mail", "sender"}, {"mail", "receiver"}, {"mail_items", "receiver"}, {"petition", "ownerguid"}, {"petition_sign", "ownerguid"}, {"petition_sign", "playerguid"}, {"pvpstats_players", "character_guid"}, {"quest_tracker", "character_guid"},
}

var characterOwnedStateDeletes = []string{
	"DELETE FROM character_account_data WHERE guid = ?",
	"DELETE FROM character_achievement WHERE guid = ?",
	"DELETE FROM character_achievement_progress WHERE guid = ?",
	"DELETE FROM character_action WHERE guid = ?",
	"DELETE FROM character_arena_stats WHERE guid = ?",
	"DELETE FROM character_aura WHERE guid = ?",
	"DELETE FROM character_banned WHERE guid = ?",
	"DELETE FROM character_battleground_data WHERE guid = ?",
	"DELETE FROM character_battleground_random WHERE guid = ?",
	"DELETE FROM character_declinedname WHERE guid = ?",
	"DELETE FROM character_equipmentsets WHERE guid = ?",
	"DELETE FROM character_fishingsteps WHERE guid = ?",
	"DELETE FROM character_gifts WHERE guid = ?",
	"DELETE FROM character_glyphs WHERE guid = ?",
	"DELETE FROM character_homebind WHERE guid = ?",
	"DELETE FROM character_instance WHERE guid = ?",
	"DELETE FROM character_inventory WHERE guid = ?",
	"DELETE FROM character_queststatus WHERE guid = ?",
	"DELETE FROM character_queststatus_daily WHERE guid = ?",
	"DELETE FROM character_queststatus_monthly WHERE guid = ?",
	"DELETE FROM character_queststatus_rewarded WHERE guid = ?",
	"DELETE FROM character_queststatus_seasonal WHERE guid = ?",
	"DELETE FROM character_queststatus_weekly WHERE guid = ?",
	"DELETE FROM character_reputation WHERE guid = ?",
	"DELETE FROM character_skills WHERE guid = ?",
	"DELETE FROM character_social WHERE guid = ? OR friend = ?",
	"DELETE FROM character_spell WHERE guid = ?",
	"DELETE FROM character_spell_cooldown WHERE guid = ?",
	"DELETE FROM character_stats WHERE guid = ?",
	"DELETE FROM character_talent WHERE guid = ?",
	"DELETE FROM character_pet_declinedname WHERE owner = ?",
	"DELETE FROM pet_aura WHERE guid IN (SELECT id FROM character_pet WHERE owner = ?)",
	"DELETE FROM pet_spell WHERE guid IN (SELECT id FROM character_pet WHERE owner = ?)",
	"DELETE FROM pet_spell_cooldown WHERE guid IN (SELECT id FROM character_pet WHERE owner = ?)",
	"DELETE FROM character_pet WHERE owner = ?",
	"DELETE FROM item_instance WHERE owner_guid = ?",
	"DELETE FROM item_refund_instance WHERE player_guid = ?",
	"DELETE FROM corpse WHERE guid = ?",
	"DELETE FROM battleground_deserters WHERE guid = ?",
	"DELETE FROM gm_survey WHERE guid = ?",
	"DELETE FROM gm_ticket WHERE playerGuid = ?",
	"DELETE FROM guild_bank_eventlog WHERE PlayerGuid = ?",
	"DELETE FROM guild_eventlog WHERE PlayerGuid1 = ? OR PlayerGuid2 = ?",
	"UPDATE characters_npcbot SET owner = 0 WHERE owner = ?",
}

func highestCharacterGUID(ctx context.Context, db *sql.DB) (uint64, error) {
	var highest uint64
	for _, ref := range characterGUIDReferences {
		var value uint64
		if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX("+ref.column+"), 0) FROM "+ref.table).Scan(&value); err != nil {
			return 0, err
		}
		if value > highest {
			highest = value
		}
	}
	return highest, nil
}

func deleteCharacterOwnedState(ctx context.Context, tx *sql.Tx, guid uint64) error {
	for _, query := range characterOwnedStateDeletes {
		var err error
		if query == "DELETE FROM character_social WHERE guid = ? OR friend = ?" || query == "DELETE FROM guild_eventlog WHERE PlayerGuid1 = ? OR PlayerGuid2 = ?" {
			_, err = tx.ExecContext(ctx, query, guid, guid)
		} else {
			_, err = tx.ExecContext(ctx, query, guid)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
