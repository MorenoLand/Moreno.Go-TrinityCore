package world

import (
	"context"
	"time"
)

// sendAchievementRewardMail mirrors the mail arm of
// AchievementMgr::CompletedAchievement reward handling
// (AchievementMgr.cpp:1544-1578). When achievement_reward.Sender is set for
// the earned achievement, the player receives a mail from
// MailSender(MAIL_CREATURE, Sender): MailDraft(MailTemplateId) when a mail
// template is set, otherwise MailDraft(Subject, Body). A configured item is
// created (Item::CreateItem), saved, and attached. The subject/body default
// to the enUS columns; achievement_reward_locale overrides for non-enUS
// sessions have no Go model.
func (s *session) sendAchievementRewardMail(achievementID uint32) {
	ctx := context.Background()
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil ||
		s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.player == nil {
		return
	}
	wdb := s.server.WorldStore.DB
	cdb := s.server.CharactersStore.DB
	var sender, templateID uint32
	var subject, body string
	var itemID uint32
	if err := wdb.QueryRowContext(ctx, "SELECT Sender, MailTemplateID, Subject, Body, ItemID FROM achievement_reward WHERE ID = ?",
		achievementID).Scan(&sender, &templateID, &subject, &body, &itemID); err != nil || sender == 0 {
		return
	}
	if templateID != 0 {
		// MailDraft(mailTemplateId): subject/body stored empty, rendered
		// from MailTemplate.dbc.
		subject, body = "", ""
	}
	now := time.Now().Unix()
	var nextMailID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&nextMailID)
	if nextMailID <= 0 {
		nextMailID = 1
	}
	var itemGUIDs []uint64
	// LoadRewards (AchievementMgr.cpp:2596) zeroes the item when its template
	// is unknown; the mail still goes out without it.
	var itemExists int
	if itemID != 0 && wdb.QueryRowContext(ctx, "SELECT 1 FROM item_template WHERE entry = ?", itemID).Scan(&itemExists) == nil {
		var nextItemGUID int64
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM item_instance").Scan(&nextItemGUID)
		if nextItemGUID <= 0 {
			nextItemGUID = 1
		}
		if _, err := cdb.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, count) VALUES (?, ?, ?, 1)",
			nextItemGUID, itemID, s.playerGUID); err == nil {
			itemGUIDs = append(itemGUIDs, uint64(nextItemGUID))
		}
	}
	hasItems := 0
	if len(itemGUIDs) > 0 {
		hasItems = 1
	}
	// MailDraft::SendMailTo (Mail.cpp:187-230): MAIL_CREATURE sender that is
	// not an online GM player gets the 30-day expiry arm; checked is 0.
	_, _ = cdb.ExecContext(ctx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
		VALUES (?, 3, 41, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0)`,
		nextMailID, templateID, sender, s.playerGUID, subject, body, hasItems, now+mailSendExpireDelay(false, 0), now)
	for _, ig := range itemGUIDs {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, receiver) VALUES (?, ?, ?)", nextMailID, ig, s.playerGUID)
	}
	s.sendMailNotify(s.playerGUID)
}
