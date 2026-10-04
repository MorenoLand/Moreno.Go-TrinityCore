package world

import (
	"context"
	"time"
)

// sendLevelUpMail mirrors the level-up mail arm of Player::GiveLevel
// (Player.cpp:2768-2775): when the player reaches a level that has a row in
// mail_level_reward matching their race mask, they receive a
// MailDraft(mailReward->mailTemplateId) mail from MailSender(MAIL_CREATURE,
// mailReward->senderEntry). ObjectMgr::GetMailLevelReward (ObjectMgr.h:1298-
// 1308) returns the first row for the level whose raceMask matches.
// ObjectMgr::LoadMailLevelRewards (ObjectMgr.cpp:9123-9178) drops rows whose
// level exceeds MAX_LEVEL, whose mask holds no playable race, whose mail
// template is unknown, or whose sender creature entry does not exist, so
// those gates are applied here against the raw table.
func (s *session) sendLevelUpMail(ctx context.Context, level uint8) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil ||
		s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.Data == nil ||
		s.player == nil {
		return
	}
	wdb := s.server.WorldStore.DB
	cdb := s.server.CharactersStore.DB
	var templateID, senderEntry uint32
	if err := wdb.QueryRowContext(ctx, "SELECT mailTemplateId, senderEntry FROM mail_level_reward WHERE level = ? AND (raceMask & ?) <> 0 LIMIT 1",
		level, playerCreateMask(s.player.Race)).Scan(&templateID, &senderEntry); err != nil {
		return
	}
	if _, ok, err := s.server.Data.MailTemplate(templateID); err != nil || !ok {
		return
	}
	var creatureExists int
	if err := wdb.QueryRowContext(ctx, "SELECT 1 FROM creature_template WHERE entry = ?", senderEntry).Scan(&creatureExists); err != nil {
		return
	}
	now := time.Now().Unix()
	var money uint32
	// C++ MailDraft::prepareItems (Mail.cpp:102-104): mail template 123
	// carries 100 gold.
	if templateID == 123 {
		money = 1000000
	}
	var nextMailID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&nextMailID)
	if nextMailID <= 0 {
		nextMailID = 1
	}
	// MailDraft::SendMailTo (Mail.cpp:187-230): MAIL_CREATURE sender that is
	// not an online GM player gets the 30-day expiry arm; subject/body are
	// stored empty and rendered from MailTemplate.dbc; checked is 0.
	_, _ = cdb.ExecContext(ctx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
		VALUES (?, 3, 41, ?, ?, ?, '', '', 0, ?, ?, ?, 0, 0)`,
		nextMailID, templateID, senderEntry, s.playerGUID, now+mailSendExpireDelay(false, 0), now, money)
	s.sendMailNotify(s.playerGUID)
}
