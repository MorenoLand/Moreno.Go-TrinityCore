package world

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// permBindAllPlayers mirrors InstanceMap::PermBindAllPlayers (Map.cpp:4127),
// fired from Unit::Kill's dungeon block (Unit.cpp:11415) when a
// player-attributed kill of a CREATURE_FLAG_EXTRA_INSTANCE_BIND (0x1) creature
// lands in a raid or heroic dungeon: every non-GM player in the instance map
// without a permanent bind for this map+difficulty is bound permanently
// (BindToInstance(save, true)), gets SMSG_INSTANCE_SAVE_CREATED plus the
// calendar lockout add, and the group is bound too when its leader is inside.
// C++ additionally calls save->SetCanReset(false) on permanent binds; Go
// keeps no instance-reset scheduler (resettime is read-only data here), so
// that half has no bridge.
func (s *Server) permBindAllPlayers(ctx context.Context, mapID, instanceID, difficulty uint32) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	db := s.CharactersStore.DB
	var resetTime int64
	if err := db.QueryRowContext(ctx, "SELECT resettime FROM instance WHERE id = ?", instanceID).Scan(&resetTime); err != nil {
		// Reference: C++ logs when the instance save is missing and binds nothing.
		if s.Logger != nil {
			s.Logger.Warn("perm bind skipped: no instance save", "instance", instanceID, "error", err)
		}
		return
	}
	s.sessionsMu.RLock()
	targets := make([]*session, 0, 16)
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID &&
			sess.player.InstanceID == instanceID && sess.player.ExtraFlags&playerExtraGMOn == 0 {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range targets {
		// Reference: the bind && bind->perm arm skips already-permanently-bound
		// players (C++ only error-logs a save mismatch, which Go cannot
		// observe — the bind table keys on instance id directly).
		var permanent int64
		var extendState int64
		var instanceRow int64
		err := db.QueryRowContext(ctx, `SELECT ci.permanent, ci.extendState, ci.instance FROM character_instance ci
			JOIN instance i ON i.id = ci.instance
			WHERE ci.guid = ? AND i.map = ? AND i.difficulty = ? LIMIT 1`,
			sess.playerGUID, mapID, difficulty).Scan(&permanent, &extendState, &instanceRow)
		if err == nil && permanent != 0 {
			continue
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			if s.Logger != nil {
				s.Logger.Warn("perm bind check failed", "account", sess.accountName, "error", err)
			}
			continue
		}
		// Reference: Player::BindToInstance(save, true, EXTEND_STATE_KEEP) —
		// the KEEP flag resolves to the player's existing extend state on an
		// update, EXTEND_STATE_NORMAL (1) on a fresh bind (Player.cpp:19121).
		if err == nil {
			if _, err := db.ExecContext(ctx, "UPDATE character_instance SET instance = ?, permanent = 1, extendState = ? WHERE guid = ? AND instance = ?",
				instanceID, extendState, sess.playerGUID, instanceRow); err != nil && s.Logger != nil {
				s.Logger.Warn("perm bind update failed", "account", sess.accountName, "error", err)
				continue
			}
		} else {
			if _, err := s.CharactersStore.ExecStatement(ctx, "CHAR_INS_CHAR_INSTANCE", sess.playerGUID, instanceID, true, uint8(1)); err != nil && s.Logger != nil {
				s.Logger.Warn("perm bind insert failed", "account", sess.accountName, "error", err)
				continue
			}
		}
		buf := protocol.NewBuffer(4)
		buf.WriteU32(0)
		_ = sess.write(uint16(protocol.OpcodeSMSG_INSTANCE_SAVE_CREATED), buf.Bytes(), true)
		// SendCalendarRaidLockout takes seconds remaining
		// (CalendarHandler.cpp:831: save->GetResetTime() - currTime), not
		// the absolute reset timestamp from the instance table.
		var remaining uint32
		if rt := resetTime - time.Now().Unix(); rt > 0 {
			remaining = uint32(rt)
		}
		sess.sendCalendarRaidLockout(mapID, difficulty, remaining, uint64(instanceID), true)
		// Reference: "if group leader is in instance, group also gets bound"
		// (Map.cpp:4168-4170) — group->BindToInstance(save, true).
		if sess.groupID != 0 {
			if group := s.findGroupByID(sess.groupID); group != nil && group.DBID != 0 && group.isLeader(sess.playerGUID) {
				if _, err := s.CharactersStore.ExecStatement(ctx, "CHAR_REP_GROUP_INSTANCE", group.DBID, instanceID, true); err != nil && s.Logger != nil {
					s.Logger.Warn("perm group bind failed", "account", sess.accountName, "error", err)
				}
			}
		}
	}
}

// maybePermBindInstance mirrors the dungeon gate of Unit::Kill's
// instance-binding block (Unit.cpp:11404-11425): only a player-attributed
// kill (the Go funnel always carries the attacker session; pet kills arrive
// through the owner's session) of an INSTANCE_BIND-flagged creature inside a
// raid or heroic dungeon fires PermBindAllPlayers. Non-dungeon kills only
// extend the save's reset time, which Go's read-only resettime model does not
// schedule — documented, no bridge.
func (s *session) maybePermBindInstance(ctx context.Context, target combatTarget, creatureEntry uint32) {
	if s == nil || s.server == nil || s.player == nil || s.playerGUID == 0 {
		return
	}
	if target.InstanceID == 0 {
		return
	}
	mapEntry, found, mapErr := s.server.Data.Map(target.Map)
	if mapErr != nil || !found || !mapEntry.IsDungeon() {
		return
	}
	// Reference: instanceMap->IsRaidOrHeroicDungeon() — the victim's
	// instance save carries the difficulty.
	var difficulty uint32
	var flagsExtra uint32
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return
	}
	if err := cdb.QueryRowContext(ctx, "SELECT difficulty FROM instance WHERE id = ?", target.InstanceID).Scan(&difficulty); err != nil {
		return
	}
	if err := wdb.QueryRowContext(ctx, "SELECT flags_extra FROM creature_template WHERE entry = ?", creatureEntry).Scan(&flagsExtra); err != nil {
		return
	}
	if !(mapEntry.IsRaid() || difficulty == 1) {
		return
	}
	if flagsExtra&0x1 == 0 {
		return
	}
	s.server.permBindAllPlayers(ctx, target.Map, target.InstanceID, difficulty)
}
