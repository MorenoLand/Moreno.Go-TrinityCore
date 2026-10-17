package world

import (
	"context"
	"database/sql"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Channel persistence: the Go port of ChannelMgr::Load / SaveToDB
// (ChannelMgr.cpp) and Channel::UpdateChannelInDB (Channel.cpp), gated by
// the PreserveCustomChannels / PreserveCustomChannelDuration /
// PreserveCustomChannelInterval worldserver.conf keys (World.cpp:685-687).
// The characters `channels` table schema already exists in
// sql/mysql/characters.sql and sql/sqlite/characters.sql.

// channelPersistTeam maps the channel creator to the C++ channels.team
// value (TeamId: 0 alliance / 1 horde). Cross-faction (two-side) channels
// land in the alliance ChannelMgr in C++ (ChannelMgr::forTeam), hence team 0.
func channelPersistTeam(s *session) uint32 {
	if s == nil || s.player == nil || s.twoSideChannelInteraction() {
		return 0
	}
	if playerTeam(s.player.Race) == teamHorde {
		return 1
	}
	return 0
}

// persistedChannelKey rebuilds the in-memory channel key for a DB row,
// matching scopedChannelKey for the row's team: the key prefix is the
// playerTeam value (469 alliance / 67 horde), not the C++ TeamId. Like
// scopedChannelKey, the prefix is dropped when
// AllowTwoSideInteractionChannel shares the key space (ChannelMgr::forTeam,
// ChannelMgr.cpp:108-121).
func (s *Server) persistedChannelKey(name string, team uint32) string {
	key := channelKey(name)
	if s != nil && s.Config.AllowTwoSideInteractionChannel {
		return key
	}
	prefix := teamAlliance
	if team == 1 {
		prefix = teamHorde
	}
	return strconv.FormatUint(uint64(prefix), 10) + ":" + key
}

// loadCustomChannels mirrors ChannelMgr::Load (ChannelMgr.cpp:30-105): the
// old-channel purge (CHAR_DEL_OLD_CHANNELS) followed by the channels-table
// SELECT. Invalid-UTF8 or invalid-team rows are deleted like C++ does.
// Runs at worldserver startup before any session attaches.
func (s *Server) loadCustomChannels(ctx context.Context) {
	if s == nil {
		return
	}
	if !s.Config.PreserveCustomChannels {
		if s.Logger != nil {
			s.Logger.Info("Loaded 0 custom chat channels. Custom channel saving is disabled.")
		}
		return
	}
	if s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	db := s.CharactersStore.DB
	now := time.Now().Unix()
	// Reference: ChannelMgr::Load — the purge only runs when the duration
	// config is non-zero (C++: `if (uint32 days = ...)`).
	if days := s.Config.PreserveCustomChannelDuration; days > 0 {
		if _, err := db.ExecContext(ctx, "DELETE FROM channels WHERE ownership = 1 AND lastUsed + ? < ?", int64(days)*86400, now); err != nil && s.Logger != nil {
			s.Logger.Warn("custom channel purge failed", "error", err)
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT name, team, announce, ownership, password, bannedList FROM channels")
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("custom channel load failed", "error", err)
		}
		return
	}
	defer rows.Close()
	type badRow struct {
		name string
		team uint32
	}
	var toDelete []badRow
	count := 0
	for rows.Next() {
		var name string
		var team uint32
		var announce, ownership int64
		var password, bannedList sql.NullString
		if err := rows.Scan(&name, &team, &announce, &ownership, &password, &bannedList); err != nil {
			if s.Logger != nil {
				s.Logger.Warn("custom channel row scan failed", "error", err)
			}
			continue
		}
		if !utf8.ValidString(name) {
			if s.Logger != nil {
				s.Logger.Warn("custom channel deleted: invalid utf8 name", "name", name)
			}
			toDelete = append(toDelete, badRow{name, team})
			continue
		}
		if team != 0 && team != 1 {
			if s.Logger != nil {
				s.Logger.Warn("custom channel deleted: invalid team", "name", name, "team", team)
			}
			toDelete = append(toDelete, badRow{name, team})
			continue
		}
		ch := &worldChannel{
			Name:             name,
			Flags:            channelFlagCustom,
			Password:         password.String,
			Announce:         announce != 0,
			Team:             team,
			OwnershipEnabled: ownership != 0,
			Members:          make(map[*session]struct{}),
			Moderators:       make(map[uint64]struct{}),
			Muted:            make(map[uint64]struct{}),
			Banned:           make(map[uint64]struct{}),
		}
		for _, field := range strings.Fields(bannedList.String) {
			if guid, err := strconv.ParseUint(field, 10, 64); err == nil && guid != 0 {
				ch.Banned[guid] = struct{}{}
			}
		}
		s.channelsMu.Lock()
		if s.channels == nil {
			s.channels = make(map[string]*worldChannel)
		}
		s.channels[s.persistedChannelKey(name, team)] = ch
		s.channelsMu.Unlock()
		count++
	}
	for _, bad := range toDelete {
		if _, err := db.ExecContext(ctx, "DELETE FROM channels WHERE name = ? AND team = ?", bad.name, bad.team); err != nil && s.Logger != nil {
			s.Logger.Warn("custom channel delete failed", "error", err)
		}
	}
	if s.Logger != nil {
		s.Logger.Info("loaded custom chat channels", "count", count)
	}
}

// runChannelSaveLoop mirrors the WUPDATE_CHANNEL_SAVE arm (World.cpp:2335-2348):
// every PreserveCustomChannelInterval minutes both team managers flush.
func (s *Server) runChannelSaveLoop(ctx context.Context) {
	if s == nil {
		return
	}
	interval := s.Config.PreserveCustomChannelInterval
	if interval == 0 {
		interval = 5
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.saveCustomChannels(ctx)
		}
	}
}

// saveCustomChannels mirrors ChannelMgr::SaveToDB calling
// Channel::UpdateChannelInDB (Channel.cpp:113-148) on every custom channel:
// dirty channels get the full row upsert (CHAR_UPD_CHANNEL), non-dirty
// channels with members get a lastUsed bump (CHAR_UPD_CHANNEL_USAGE) once
// their activity-update timer elapses. REPLACE INTO is the portable
// equivalent of C++'s INSERT ... ON DUPLICATE KEY UPDATE (see
// upsertSocialContact in social.go); all row columns are supplied in both.
func (s *Server) saveCustomChannels(ctx context.Context) {
	if s == nil || !s.Config.PreserveCustomChannels {
		return
	}
	if s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	db := s.CharactersStore.DB
	now := time.Now()
	interval := s.Config.PreserveCustomChannelInterval
	if interval == 0 {
		interval = 5
	}
	s.channelsMu.Lock()
	defer s.channelsMu.Unlock()
	for _, ch := range s.channels {
		if ch == nil || ch.Flags&channelFlagCustom == 0 {
			continue
		}
		flushed := false
		if ch.Dirty {
			guids := make([]uint64, 0, len(ch.Banned))
			for guid := range ch.Banned {
				guids = append(guids, guid)
			}
			sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
			var banned strings.Builder
			for _, guid := range guids {
				banned.WriteString(strconv.FormatUint(guid, 10))
				banned.WriteByte(' ')
			}
			announce, ownership := 0, 0
			if ch.Announce {
				announce = 1
			}
			if ch.OwnershipEnabled {
				ownership = 1
			}
			if _, err := db.ExecContext(ctx, "REPLACE INTO channels (name, team, announce, ownership, password, bannedList, lastUsed) VALUES (?, ?, ?, ?, ?, ?, ?)",
				ch.Name, ch.Team, announce, ownership, ch.Password, banned.String(), now.Unix()); err != nil {
				if s.Logger != nil {
					s.Logger.Warn("custom channel save failed", "channel", ch.Name, "error", err)
				}
				continue
			}
			flushed = true
		} else if !ch.NextActivityUpdate.After(now) {
			// Reference: UpdateChannelInDB's non-dirty arm — the usage bump
			// only fires when the channel still has members.
			if len(ch.Members) == 0 {
				continue
			}
			if _, err := db.ExecContext(ctx, "UPDATE channels SET lastUsed = ? WHERE name = ? AND team = ?", now.Unix(), ch.Name, ch.Team); err != nil {
				if s.Logger != nil {
					s.Logger.Warn("custom channel usage save failed", "channel", ch.Name, "error", err)
				}
				continue
			}
			flushed = true
		}
		if !flushed {
			continue
		}
		ch.Dirty = false
		// Reference: _nextActivityUpdateTime = now + urand(1min, 6min) *
		// max(1, PreserveCustomChannelInterval).
		mult := interval
		if mult < 1 {
			mult = 1
		}
		ch.NextActivityUpdate = now.Add(time.Duration(1+rand.IntN(6)) * time.Minute * time.Duration(mult))
	}
}
