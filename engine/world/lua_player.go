package world

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func (s *Server) luaPlayers() []*scripting.Object {
	s.sessionsMu.RLock()
	players := make([]*scripting.Object, 0, len(s.sessions))
	for value := range s.sessions {
		if value.worldReady.Load() {
			if player := value.luaPlayer(); player != nil {
				players = append(players, player)
			}
		}
	}
	s.sessionsMu.RUnlock()
	return players
}

func (s *session) triggerPlayerEvent(ctx context.Context, event int, args ...any) {
	_, _ = s.triggerPlayerEventValues(ctx, event, args...)
}

func (s *session) triggerPlayerEventValues(ctx context.Context, event int, args ...any) ([]any, error) {
	if s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return nil, nil
	}
	values, err := s.server.Features.Scripts.TriggerPlayerEvent(ctx, event, append([]any{event}, args...)...)
	if err != nil {
		s.debug("lua player event failed", "account", s.accountName, "event", event, "error", err)
	}
	return values, err
}

func (s *session) triggerPlayerEventValues2(ctx context.Context, event int, args ...any) ([][]any, error) {
	if s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return nil, nil
	}
	values, err := s.server.Features.Scripts.TriggerPlayerEvent2(ctx, event, append([]any{event}, args...)...)
	if err != nil {
		s.debug("lua player event failed", "account", s.accountName, "event", event, "error", err)
	}
	return values, err
}

func (s *Server) findPlayer(guid uint64) *scripting.Object {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for value := range s.sessions {
		if value.worldReady.Load() && value.playerGUID == guid {
			return value.luaPlayer()
		}
	}
	return nil
}

// luaPlayerOrNil is the nil-session-safe twin of luaPlayer: a missing
// session yields nil, the Eluna::Push((Player*)nullptr) = nil twin.
func luaPlayerOrNil(s *session) *scripting.Object {
	if s == nil {
		return nil
	}
	return s.luaPlayer()
}

func (s *session) luaPlayer() *scripting.Object {
	if !s.playerLoaded || s.player == nil {
		return nil
	}
	state := s.player
	powerType := playerPowerType(state)
	fields := map[string]any{"Name": state.Name, "GUID": state.GUID, "GUIDLow": uint32(state.GUID), "MapId": state.Map, "InstanceId": state.InstanceID, "Level": state.Level, "Race": state.Race, "Class": state.Class, "Gender": state.Gender, "Team": teamForRace(state.Race), "IsGM": s.security > 0, "InWorld": true, "X": state.X, "Y": state.Y, "Z": state.Z, "Orientation": state.Orientation, "Zone": state.Zone, "Health": state.Health, "MaxHealth": state.MaxHealth, "Power": state.Powers[powerType], "MaxPower": state.MaxPowers[powerType], "PowerType": powerType, "InCombat": s.attackTarget != 0 || state.UnitFlags&unitFlagInCombat != 0}
	methods := map[string]scripting.ObjectMethod{}
	methods["GetName"] = luaNoArgs(func() any { return state.Name })
	methods["GetGUID"] = luaNoArgs(func() any { return state.GUID })
	methods["GetGUIDLow"] = luaNoArgs(func() any { return uint32(state.GUID) })
	methods["GetMapId"] = luaNoArgs(func() any { return state.Map })
	methods["GetInstanceId"] = luaNoArgs(func() any { return state.InstanceID })
	// GetPhaseMask is WorldObject::GetPhaseMask (WorldObjectMethods.h):
	// the player's derived phase mask (phase-aura union, PHASEMASK_NORMAL
	// when none, all phases for GMs — currentPlayerPhaseMask in death.go).
	// There is no player SetPhaseMask twin: C++ player phase is derived
	// by the PhaseMgr from auras/conditions, not stored, so a direct set
	// would be overwritten; it stays out.
	methods["GetPhaseMask"] = luaNoArgs(func() any { return s.currentPlayerPhaseMask() })
	// GetMap is WorldObject::GetMap: the DBC-backed Map object for the
	// player's map/instance (NewMapObject + mapLive, the same
	// builder behind the creature surface and the instance hooks' map
	// argument). Nil when the server store is unavailable.
	methods["GetMap"] = func(_ context.Context, _ []any) ([]any, error) {
		if s.server == nil || s.server.Data == nil {
			return []any{nil}, nil
		}
		return []any{scripting.NewMapObject(state.Map, state.InstanceID, s.server.mapLive(state.Map, state.InstanceID))}, nil
	}
	methods["GetObjectType"] = luaNoArgs(func() any { return "Player" })
	methods["GetZ"] = luaNoArgs(func() any { return state.Z })
	methods["GetLevel"] = luaNoArgs(func() any { return state.Level })
	methods["GetRace"] = luaNoArgs(func() any { return state.Race })
	methods["GetClass"] = luaNoArgs(func() any { return state.Class })
	// GetClassMask/GetRaceMask are Unit::GetClassMask/GetRaceMask
	// (Unit.h:849/852). IsArmorer/IsBanker/IsBattleMaster/IsGuildMaster/
	// IsTabardDesigner are the Unit.h:1011-1019 UNIT_NPC_FLAGS reads
	// (REPAIR 0x1000, BANKER 0x20000, BATTLEMASTER 0x100000, PETITIONER
	// 0x40000, TABARDDESIGNER 0x80000); players carry no NPC flags, so
	// all five read false. IsPvPFlagged is Unit::IsPvP (Unit.h:914), the
	// UNIT_FIELD_BYTES_2 PvP byte flag, the player PVPFlags bit in Go.
	methods["GetClassMask"] = luaNoArgs(func() any { return unitClassRaceMask(uint32(state.Class)) })
	methods["GetRaceMask"] = luaNoArgs(func() any { return unitClassRaceMask(uint32(state.Race)) })
	// GetClassAsString/GetRaceAsString are Unit::GetClassAsString/
	// GetRaceAsString (UnitMethods.h:1152/1195): ChrClasses/ChrRaces
	// Name[locale] for the player's class/race, nil on a missing row.
	methods["GetClassAsString"] = func(_ context.Context, args []any) ([]any, error) {
		locale, err := dbcLocaleArg(args)
		if err != nil {
			return nil, err
		}
		name, err := s.server.dbcLocaleName("ChrClasses", uint32(state.Class), 4, locale)
		if err != nil {
			return nil, err
		}
		return []any{name}, nil
	}
	methods["GetRaceAsString"] = func(_ context.Context, args []any) ([]any, error) {
		locale, err := dbcLocaleArg(args)
		if err != nil {
			return nil, err
		}
		name, err := s.server.dbcLocaleName("ChrRaces", uint32(state.Race), 14, locale)
		if err != nil {
			return nil, err
		}
		return []any{name}, nil
	}
	// GetControllerGUID/GetControllerGUIDS are Unit::GetCharmerOrOwnerGUID
	// (Unit.h:1170) and WorldObject::GetCharmerOrOwnerOrOwnGUID
	// (Object.cpp:2196): charmer else owner, else the unit's own GUID.
	// Go stores no charmer or owner for players, so the first reads 0
	// and the second reads the player's own GUID.
	methods["GetControllerGUID"] = luaNoArgs(func() any { return uint64(0) })
	methods["GetControllerGUIDS"] = luaNoArgs(func() any { return state.GUID })
	// GetBaseSpellPower is Unit::GetBaseSpellPower (UnitMethods.h:743):
	// the PLAYER_FIELD_MOD_DAMAGE_DONE_POS + school field. C++ writes
	// that field only for schools HOLY..SHADOW
	// (Player::UpdateSpellDamageAndHealingBonus, StatSystem.cpp): POS
	// is SpellBaseDamageBonusDone(1<<school) minus the NEG field, the
	// sum of negative MOD_DAMAGE_DONE amounts for the school — so the
	// Go twin folds the live aura pipeline the same way. School 0
	// (NORMAL) is never rewritten after the creation reset, so it
	// reads 0; school >= MAX_SPELL_SCHOOL (7) returns nothing in C++
	// (return 1 with no push), which Go renders as one nil value.
	// Creatures carry no player fields, so their surfaces read 0.
	methods["GetBaseSpellPower"] = func(_ context.Context, args []any) ([]any, error) {
		school, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if school >= 7 {
			return []any{nil}, nil
		}
		if school == 0 {
			return []any{uint32(0)}, nil
		}
		mask := uint32(1) << school
		base := s.spellBaseDamageBonusDone(mask)
		neg := int32(0)
		for _, amt := range s.auraTypeModifiersByMiscMask(spellAuraModDamageDone, mask) {
			if amt < 0 {
				neg += amt
			}
		}
		return []any{uint32(base - neg)}, nil
	}
	methods["IsArmorer"] = luaNoArgs(func() any { return false })
	methods["IsBanker"] = luaNoArgs(func() any { return false })
	methods["IsBattleMaster"] = luaNoArgs(func() any { return false })
	methods["IsGuildMaster"] = luaNoArgs(func() any { return false })
	methods["IsTabardDesigner"] = luaNoArgs(func() any { return false })
	methods["IsPvPFlagged"] = luaNoArgs(func() any { return state.PVPFlags&pvpFlagPvP != 0 })
	// IsAttackingPlayer is Unit::isAttackingPlayer (Unit.cpp:5849-5865):
	// the UNIT_STATE_ATTACK_PLAYER read, which Go models as the
	// contested-PvP flag (threat.go SetContestedPvP twin). The pure
	// query form reads the flag without running the expiry side effect;
	// the world tick's updateContestedPvP owns expiry, like C++'s
	// UpdateContestedPvP. The creature-surface twin lives in
	// lua_worldobject_range.go.
	methods["IsAttackingPlayer"] = luaNoArgs(func() any { return s.contestedPvPActiveQuery(time.Now()) })
	// SendChatMessageToPlayer is UnitMethods::SendChatMessageToPlayer
	// (UnitMethods.h:2095-2113), shared with the creature surfaces in
	// lua_worldobject_range.go; the sender chat tag is the player's
	// GetChatTag value, like C++ BuildChatPacket's player-sender arm.
	methods["SendChatMessageToPlayer"] = func(_ context.Context, args []any) ([]any, error) {
		return s.server.luaSendChatMessageToPlayer(state.GUID, s.chatTag(), args)
	}
	methods["GetGender"] = luaNoArgs(func() any { return state.Gender })
	// GetDisplayId is Unit::GetDisplayId (Unit.h): the UNIT_FIELD_DISPLAYID
	// read. Go computes the same value the update-values builder broadcasts
	// (player_state.go): race male/female display by gender, overridden by
	// the active transform display. Backs the maladaar stolen-soul and
	// kiljaeden sinister-reflection SetDisplayId arms.
	methods["GetDisplayId"] = luaNoArgs(func() any { return playerLuaDisplayID(s, state) })
	// GetNativeDisplayId is Unit::GetNativeDisplayId (Unit.h:1474):
	// the race male/female display, or the SetNativeDisplayId
	// override while set (playerLuaNativeDisplayID, shared with the
	// values builder).
	methods["GetNativeDisplayId"] = luaNoArgs(func() any { return playerLuaNativeDisplayID(s, state) })
	// SetNativeDisplayId is Unit::SetNativeDisplayId (Unit.h:1476):
	// the native field only, session-scoped like the faction/talent
	// overrides; the live display is untouched until DeMorph.
	methods["SetNativeDisplayId"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		state.NativeDisplayIDOverride = id
		state.NativeDisplayIDOverrideSet = true
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	// DeMorph is Unit::DeMorph (Unit.cpp:3321-3323): SetDisplayId
	// of the native display. Go display reads the transform display
	// while set, else the race display, so DeMorph clears the
	// transform or, with a native override, points the transform at
	// it. DELTA: a shapeshift form display is a transform too, so
	// DeMorph drops it until the next shapeshift recalc; C++ keeps
	// the form.
	methods["DeMorph"] = func(_ context.Context, _ []any) ([]any, error) {
		if state.NativeDisplayIDOverrideSet {
			state.TransformDisplayID = state.NativeDisplayIDOverride
		} else {
			state.TransformDisplayID = 0
		}
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	// GetCreatureType is Unit::GetCreatureType (Unit.cpp:9124-9149)
	// for players: the shapeshift form's CreatureType while shifted
	// (SpellShapeshiftForm.dbc field 20, >0 only), else the race
	// CreatureType (ChrRaces.dbc field 8); a missing DBC row reads 0.
	methods["GetCreatureType"] = func(_ context.Context, _ []any) ([]any, error) {
		if state.ShapeshiftForm != 0 && s.server != nil && s.server.Data != nil {
			if shape, found, err := s.server.Data.ShapeshiftForm(uint32(state.ShapeshiftForm)); err == nil && found && shape.CreatureType > 0 {
				return []any{uint32(shape.CreatureType)}, nil
			}
		}
		if s.server != nil && s.server.Data != nil {
			if race, found, err := s.server.Data.Race(uint32(state.Race)); err == nil && found {
				return []any{race.CreatureType}, nil
			}
		}
		return []any{uint32(0)}, nil
	}
	// GetNativeGender is Player::GetNativeGender (Player.h:993): the
	// PLAYER_BYTES_3 gender byte. Go tracks a single gender
	// (characters.gender, the same field GetGender reads); no
	// display-gender transform is modeled, so native == current here.
	// Backs the outhouse bunny SetData arm (go_scripts.cpp:615-641).
	methods["GetNativeGender"] = luaNoArgs(func() any { return state.Gender })
	methods["GetDbcLocale"] = luaNoArgs(func() any { return uint32(0) })
	methods["GetPowerType"] = luaNoArgs(func() any { return playerPowerType(state) })
	methods["GetCoinage"] = luaNoArgs(func() any { return state.Money })
	methods["GetGuildId"] = luaNoArgs(func() any { return state.GuildID })
	// GetCorpse is Player::GetCorpse (PlayerMethods.h:1581-1588): the
	// player's resurrectable corpse object, or nil when the player has
	// none (never died, corpse reclaimed, or a BG/arena death whose corpse
	// Go never persists). The ghost/hasCorpse gate mirrors C++ m_corpse
	// lifetime: a non-bones resurrect despawns the visual but leaves the
	// row behind, so row-presence alone would return a corpse for a live
	// player.
	methods["GetCorpse"] = func(ctx context.Context, _ []any) ([]any, error) {
		if s.player == nil || (s.player.PlayerFlags&playerFlagGhost == 0 && !s.player.hasCorpse) {
			return []any{nil}, nil
		}
		corpse, ok := s.loadCorpseObject(ctx)
		if !ok {
			return []any{nil}, nil
		}
		return []any{s.luaCorpseObject(corpse)}, nil
	}
	// GetTeam/IsAlliance/IsHorde are Player::GetTeamId (Player.h:1759)
	// via effectiveTeamFor, the Go form of Player::TeamForRace
	// (Player.cpp:6619): ChrRaces BaseLanguage 7 = Alliance, 1 = Horde,
	// with a SetFactionForRace override standing in for the race.
	methods["GetTeam"] = luaNoArgs(func() any { return effectiveTeamFor(state) })
	methods["IsAlliance"] = luaNoArgs(func() any { return effectiveTeamFor(state) == 0 })
	methods["IsHorde"] = luaNoArgs(func() any { return effectiveTeamFor(state) == 1 })
	// SetFactionForRace is Player::SetFactionForRace (Player.cpp:6636):
	// m_team + UNIT_FIELD_FACTIONTEMPLATE from the GIVEN race's
	// ChrRaces row (0 when the row is missing). Go stores the race
	// as a session-lifetime override: effectiveFactionTemplate and
	// effectiveTeamFor read it, and login recomputes from the player's
	// own race (Player.cpp:17517), so it does not persist. Team-gated
	// subsystems that call teamForRace(state.Race) directly (taxi,
	// chat, reputation) keep the race model.
	methods["SetFactionForRace"] = func(_ context.Context, args []any) ([]any, error) {
		race, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		state.FactionRaceOverride = uint8(race)
		state.FactionRaceOverrideSet = true
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	// IsInGuild is Player::GetGuildId() != 0 (PlayerMethods.h:400-404).
	methods["IsInGuild"] = luaNoArgs(func() any { return state.GuildID != 0 })
	// GetGuildRank is Player::GetRank (PlayerMethods.h:1152-1156), the
	// guild member rank the guild code maintains on the player state.
	methods["GetGuildRank"] = luaNoArgs(func() any { return state.GuildRank })
	// GetTotalPlayedTime is Player::GetTotalPlayedTime
	// (PlayerMethods.h:1524-1528), seconds, advanced on save.
	methods["GetTotalPlayedTime"] = luaNoArgs(func() any { return state.TotalPlayedTime })
	// IsAFK/IsDND are Player::isAFK/isDND (PlayerMethods.h:501-516), the
	// PLAYER_FLAGS bits (AFK 0x2, DND 0x4).
	methods["IsAFK"] = luaNoArgs(func() any { return state.PlayerFlags&playerFlagAFK != 0 })
	methods["IsDND"] = luaNoArgs(func() any { return state.PlayerFlags&playerFlagDND != 0 })
	// GetChatTag is Player::GetChatTag (PlayerMethods.h:1328-1332;
	// Player.cpp:1642-1654): the AFK/DND/GM bits of the chat.go tag
	// helper, DEV never set (no developer-flag model in Go).
	methods["GetChatTag"] = luaNoArgs(func() any { return s.chatTag() })
	// ToggleAFK/ToggleDND are Player::ToggleAFK/ToggleDND
	// (Player.cpp:1628-1640): the flag flip plus client update via the
	// chat helpers. The battleground-leave half of ToggleAFK is not in
	// the Lua path (same split as setPlayerAFK in chat.go).
	methods["ToggleAFK"] = func(_ context.Context, _ []any) ([]any, error) {
		s.setPlayerAFK(state.PlayerFlags&playerFlagAFK == 0)
		return []any{}, nil
	}
	methods["ToggleDND"] = func(_ context.Context, _ []any) ([]any, error) {
		s.setPlayerDND(state.PlayerFlags&playerFlagDND == 0)
		return []any{}, nil
	}
	// IsAcceptingWhispers/IsGMChat/IsGMVisible/IsTaxiCheater are the
	// Player::isAcceptWhispers/isGMChat/isGMVisible/isTaxiCheater
	// ExtraFlags tests (Player.h:957-967): ACCEPT_WHISPERS 0x4,
	// TAXICHEAT 0x8, GM_CHAT 0x20, GM_INVISIBLE 0x10 (inverted).
	methods["IsAcceptingWhispers"] = luaNoArgs(func() any { return state.ExtraFlags&playerExtraAcceptWhispers != 0 })
	methods["IsGMChat"] = luaNoArgs(func() any { return state.ExtraFlags&playerExtraGMChat != 0 })
	methods["IsGMVisible"] = luaNoArgs(func() any { return state.ExtraFlags&playerExtraGMInvisible == 0 })
	methods["IsTaxiCheater"] = luaNoArgs(func() any { return state.ExtraFlags&playerExtraTaxiCheat != 0 })
	// extraBool mirrors Eluna CHECKVAL<bool>(L, 2, true): an omitted
	// argument means true, a non-boolean argument is an error.
	extraBool := func(method string, args []any) (bool, error) {
		if len(args) == 0 {
			return true, nil
		}
		on, ok := args[0].(bool)
		if !ok {
			return false, fmt.Errorf("%s requires a boolean", method)
		}
		return on, nil
	}
	// SetSanctuary is UnitMethods::SetSanctuary (UnitMethods.h:1892-1908):
	// the UNIT_BYTE2_FLAG_SANCTUARY (0x08, UnitDefines.h:106) byte flag
	// on UNIT_FIELD_BYTES_2 byte 1 — Go's player PVPFlags byte, which
	// the update-values builder already folds into bytes2 — plus the
	// CombatStop() arm (includingCast=false, mutualPvP=true, Unit.h:823):
	// attack stop + in-combat clear, the handleCmdCombatStop twin.
	// DELTAS: the RemoveAllAttackers leg (attackers stop attacking this
	// unit) has no Go bridge, same documented gap as the command twin;
	// the CombatStopWithPets leg is unmodeled (no pet attack-stop
	// primitive that does not halt motion); the creature surface stays
	// unwired (no creature bytes2-byte1 field — motion tracks only the
	// byte-0 sheath state).
	methods["SetSanctuary"] = func(_ context.Context, args []any) ([]any, error) {
		apply, err := extraBool("SetSanctuary", args)
		if err != nil {
			return nil, err
		}
		if apply {
			state.PVPFlags |= pvpFlagSanctuary
			if s.attackTarget != 0 {
				victim := s.attackTarget
				s.attackTarget = 0
				_ = s.sendAttackStop(victim, false)
			}
			state.UnitFlags &^= unitFlagInCombat
		} else {
			state.PVPFlags &^= pvpFlagSanctuary
		}
		s.sendPlayerUpdate()
		return nil, nil
	}
	// SetAcceptWhispers/SetGMChat/SetTaxiCheat are the Player:: bit
	// flips (Player.h:958/964/966); the GM-chat setter also keeps the
	// session gmChat flag in step, the .gm chat command twin
	// (commands.go). Persistence follows persistExtraFlags.
	methods["SetAcceptWhispers"] = func(_ context.Context, args []any) ([]any, error) {
		on, err := extraBool("SetAcceptWhispers", args)
		if err != nil {
			return nil, err
		}
		if on {
			state.ExtraFlags |= playerExtraAcceptWhispers
		} else {
			state.ExtraFlags &^= playerExtraAcceptWhispers
		}
		s.persistExtraFlags()
		return []any{}, nil
	}
	methods["SetGMChat"] = func(_ context.Context, args []any) ([]any, error) {
		on, err := extraBool("SetGMChat", args)
		if err != nil {
			return nil, err
		}
		s.gmChat = on
		if on {
			state.ExtraFlags |= playerExtraGMChat
		} else {
			state.ExtraFlags &^= playerExtraGMChat
		}
		s.persistExtraFlags()
		return []any{}, nil
	}
	methods["SetTaxiCheat"] = func(_ context.Context, args []any) ([]any, error) {
		on, err := extraBool("SetTaxiCheat", args)
		if err != nil {
			return nil, err
		}
		if on {
			state.ExtraFlags |= playerExtraTaxiCheat
		} else {
			state.ExtraFlags &^= playerExtraTaxiCheat
		}
		s.persistExtraFlags()
		return []any{}, nil
	}
	// SetGameMaster is Player::SetGameMaster (Player.cpp:2439-2498)
	// via the .gm command twin (commands.go): the GM_ON bit plus the
	// PLAYER_FLAGS GM bit, the visibility-detect flip, persist, and
	// the client/visibility refresh. The faction, phase-anywhere,
	// FFA-byte, pet-faction, and combat-stop legs are the documented
	// gm-on gap shared with the command twin.
	methods["SetGameMaster"] = func(ctx context.Context, args []any) ([]any, error) {
		on, err := extraBool("SetGameMaster", args)
		if err != nil {
			return nil, err
		}
		if on {
			state.ExtraFlags |= playerExtraGMOn
			state.PlayerFlags |= playerFlagGM
			s.gmVisibilityDetect = s.security
		} else {
			state.ExtraFlags &^= playerExtraGMOn
			state.PlayerFlags &^= playerFlagGM
			s.gmVisibilityDetect = 0
		}
		s.updateWorldReadyGM()
		s.persistExtraFlags()
		s.sendPlayerUpdate()
		s.refreshNearbyObjects(ctx)
		return []any{}, nil
	}
	// SetGMVisible is Player::SetGMVisible (Player.cpp:2504-2523) via
	// the .gm visible command twin (commands.go): the invisible-bit
	// flip plus the GM visual aura, with the off arm also clearing
	// accept-whispers and turning GM mode on per C++. Channel
	// invisibility needs no per-channel action: the channel code
	// reads the invisible bit live. The SetGameMaster engine legs
	// above are the same documented gap.
	methods["SetGMVisible"] = func(ctx context.Context, args []any) ([]any, error) {
		on, err := extraBool("SetGMVisible", args)
		if err != nil {
			return nil, err
		}
		if on {
			state.ExtraFlags &^= playerExtraGMInvisible
			if s.hasAura(gmVisualAura) {
				s.removeAura(gmVisualAura)
			}
			s.gmVisibilityDetect = 0
		} else {
			state.ExtraFlags |= playerExtraGMInvisible
			state.ExtraFlags &^= playerExtraAcceptWhispers
			state.ExtraFlags |= playerExtraGMOn
			state.PlayerFlags |= playerFlagGM
			s.applyAuraWithDuration(gmVisualAura, 0)
			s.gmVisibilityDetect = s.security
		}
		s.updateWorldReadyGM()
		s.persistExtraFlags()
		s.sendPlayerUpdate()
		s.refreshNearbyObjects(ctx)
		return []any{}, nil
	}
	// HasAtLoginFlag/SetAtLoginFlag are Player::HasAtLoginFlag/
	// SetAtLoginFlag (Player.h:2025-2026): a plain bit test / OR on
	// m_atLoginFlags. The setter persists immediately (UPDATE
	// characters at_login, the characters.go:1376 shape) like the
	// other Lua setters; C++ itself only saves at the next SaveToDB.
	methods["HasAtLoginFlag"] = func(_ context.Context, args []any) ([]any, error) {
		flag, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{state.AtLogin&flag != 0}, nil
	}
	methods["SetAtLoginFlag"] = func(ctx context.Context, args []any) ([]any, error) {
		flag, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		state.AtLogin |= flag
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET at_login = ? WHERE guid = ?", state.AtLogin, s.playerGUID)
		}
		return []any{}, nil
	}
	// HasTitle/SetKnownTitle/UnsetKnownTitle are Player::HasTitle/
	// SetTitle (Player.cpp:24799-24841) behind the CharTitles DBC
	// lookup (PlayerMethods.h:144-149/2052/3295). An unknown title
	// id pushes nil (the C++ lookup miss pushes nothing) and the
	// setters no-op; id 0 counts as unknown even though the Go
	// store reports it found with an empty row. MAX_TITLE_INDEX
	// is 3*64 = 192 (Player.h:410-411): a known title above it
	// reads false, and MaskID 192 (word offset 6, past the six
	// KnownTitles words) reads false rather than the out-of-range
	// field C++ would touch. The already-known add / unknown
	// remove arms return before any broadcast, exactly like C++;
	// persistTitles broadcasts the KnownTitles fields, standing
	// in for SMSG_TITLE_EARNED, which has no Go sender.
	// ChosenTitle is untouched on both arms: C++ SetTitle never
	// moves it (load-time validation resets a stale chosen title).
	titleMask := func(id uint32) (uint32, bool) {
		if id == 0 || s.server == nil || s.server.Data == nil {
			return 0, false
		}
		title, found, err := s.server.Data.CharTitle(id)
		if err != nil || !found {
			return 0, false
		}
		return title.MaskID, true
	}
	methods["HasTitle"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		maskID, ok := titleMask(id)
		if !ok {
			return []any{nil}, nil
		}
		if maskID > 192 {
			return []any{false}, nil
		}
		idx := maskID / 32
		if int(idx) >= len(state.KnownTitles) {
			return []any{false}, nil
		}
		return []any{state.KnownTitles[idx]&(uint32(1)<<(maskID%32)) != 0}, nil
	}
	methods["SetKnownTitle"] = func(ctx context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		maskID, ok := titleMask(id)
		if !ok || maskID > 192 {
			return []any{}, nil
		}
		idx := maskID / 32
		if int(idx) >= len(state.KnownTitles) {
			return []any{}, nil
		}
		bit := uint32(1) << (maskID % 32)
		if state.KnownTitles[idx]&bit != 0 {
			return []any{}, nil
		}
		state.KnownTitles[idx] |= bit
		if s.server != nil && s.server.CharactersStore != nil {
			s.persistTitles(ctx, s)
		}
		return []any{}, nil
	}
	methods["UnsetKnownTitle"] = func(ctx context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		maskID, ok := titleMask(id)
		if !ok || maskID > 192 {
			return []any{}, nil
		}
		idx := maskID / 32
		if int(idx) >= len(state.KnownTitles) {
			return []any{}, nil
		}
		bit := uint32(1) << (maskID % 32)
		if state.KnownTitles[idx]&bit == 0 {
			return []any{}, nil
		}
		state.KnownTitles[idx] &^= bit
		if s.server != nil && s.server.CharactersStore != nil {
			s.persistTitles(ctx, s)
		}
		return []any{}, nil
	}
	// GetSpecsCount/GetActiveSpec are Player::GetSpecsCount/
	// GetActiveSpec (Player.h:1459-1461): m_specsCount and
	// m_activeSpec, the dual-spec index pair (0/1), not talent
	// tab ids. Go reads TalentGroupsCount/ActiveTalentGroup, the
	// same fields the characters save/load round-trips.
	methods["GetSpecsCount"] = luaNoArgs(func() any { return state.TalentGroupsCount })
	methods["GetActiveSpec"] = luaNoArgs(func() any { return state.ActiveTalentGroup })
	// GetDifficulty is Player::GetDifficulty (Player.h:1576): the
	// raid difficulty when isRaid (omitted means true, the Eluna
	// CHECKVAL default) else the dungeon difficulty.
	methods["GetDifficulty"] = func(_ context.Context, args []any) ([]any, error) {
		isRaid, err := extraBool("GetDifficulty", args)
		if err != nil {
			return nil, err
		}
		if isRaid {
			return []any{state.RaidDifficulty}, nil
		}
		return []any{state.DungeonDifficulty}, nil
	}
	// GetShieldBlockValue is Player::GetShieldBlockValue
	// (Player.cpp:5426-5430) via the session fold in
	// aura_shieldblock.go: max(0, (flat + str*0.5 - 10) * pct).
	methods["GetShieldBlockValue"] = luaNoArgs(func() any { return s.getShieldBlockValue() })
	// GetHealthBonusFromStamina/GetManaBonusFromIntellect are
	// Player::GetHealthBonusFromStamina/GetManaBonusFromIntellect
	// (StatSystem.cpp:286-303): min(20, stat) + (stat-20)*10 for
	// stamina, *15 for intellect, over the total stats (Stats[2]
	// stamina, Stats[3] intellect, the effectiveMaxHealth precedent).
	methods["GetHealthBonusFromStamina"] = luaNoArgs(func() any {
		if len(state.Stats) <= 2 {
			return float64(0)
		}
		return float64(staminaHealthBonus(state.Stats[2]))
	})
	methods["GetManaBonusFromIntellect"] = luaNoArgs(func() any {
		if len(state.Stats) <= 3 {
			return float64(0)
		}
		intellect := float64(state.Stats[3])
		base := math.Min(20, intellect)
		return base + (intellect-base)*15
	})
	// IsImmuneToDamage is Player::isTotalImmune (Player.cpp:24785):
	// any SPELL_AURA_SCHOOL_IMMUNITY effect whose misc OR covers a
	// school, via the session fold in immunity.go.
	methods["IsImmuneToDamage"] = luaNoArgs(func() any { return s.isTotalImmune() })
	// IsFalling is Unit::IsFalling (Unit.cpp:13313): the
	// FALLING/FALLING_FAR movement flags. The movespline-falling
	// leg has no Go model, so a spline-driven fall reads false.
	methods["IsFalling"] = luaNoArgs(func() any { return s.isFalling || s.isFallingFar })
	// CanSpeak is WorldSession::CanSpeak (WorldSession.cpp:723):
	// the mute deadline has passed. Go clears an expired mute
	// lazily in chat.go, so the comparison alone is exact.
	methods["CanSpeak"] = luaNoArgs(func() any { return !(s.muteTime > time.Now().Unix()) })
	// GetDbLocaleIndex is WorldSession::GetSessionDbLocaleIndex:
	// the session locale index from account.locale (0 = enUS).
	methods["GetDbLocaleIndex"] = luaNoArgs(func() any { return s.locale })
	// GetChampioningFaction is Player::GetChampioningFaction
	// (Player.h:2154): the m_ChampioningFaction field, set by a
	// tabard championing aura apply and cleared on remove
	// (SpellAuraEffects.cpp:4599). Go stores no field, so the
	// read scans the live championing auras in fixed spell order
	// (the kill-arm map/difficulty gates in reputation.go do not
	// apply to the field read); with several tabards C++ keeps
	// the last applied, this keeps the first in spell order.
	methods["GetChampioningFaction"] = luaNoArgs(func() any {
		for _, pair := range [][2]uint32{{57819, 1106}, {57820, 1098}, {57821, 1090}, {57822, 1091}} {
			if s.hasAura(pair[0]) {
				return pair[1]
			}
		}
		return uint32(0)
	})
	// GetPhaseMaskForSpawn is Player::GetPhaseMaskForSpawn
	// (Player.cpp:25279-25297): a non-GM reads the current phase
	// mask; a GM reads the first SPELL_AURA_PHASE aura's misc
	// value, else PHASEMASK_NORMAL; the result strips the normal
	// bit, falling back to it. Go has no stored base phase mask,
	// so the non-GM arm uses currentPlayerPhaseMask (phase-aura
	// OR, else 1) and the GM test mirrors it (PLAYER_FLAGS GM or
	// ExtraFlags GMOn).
	methods["GetPhaseMaskForSpawn"] = luaNoArgs(func() any {
		var phase uint32
		if state.PlayerFlags&playerFlagGM != 0 || state.ExtraFlags&playerExtraGMOn != 0 {
			phase = 1
			s.castMu.Lock()
			for _, aura := range s.activeAuras {
				if aura != nil && !aura.Stopped && aura.AuraType == spellAuraPhase {
					phase = uint32(aura.MiscValue)
					break
				}
			}
			s.castMu.Unlock()
		} else {
			phase = s.currentPlayerPhaseMask()
		}
		if stripped := phase &^ 1; stripped != 0 {
			return stripped
		}
		return uint32(1)
	})
	// HasFlag/SetFlag/RemoveFlag are Object::HasFlag/SetFlag/RemoveFlag
	// (ObjectMethods.h:34/243/390; LuaFunctions.cpp:1354 registers
	// ObjectMethods on the Player metatable) for the flag-backed fields
	// the session twin honors: UNIT_FIELD_FLAGS (59, UpdateFields.h:117)
	// reads state.UnitFlags, UNIT_FIELD_FLAGS_2 (60) reads
	// state.UnitFlags2, PLAYER_FLAGS (150, UpdateFields.h:178) reads
	// state.PlayerFlags. Other fields have no player backing: HasFlag
	// reads false and writes are ignored (creature HasFlag precedent);
	// the twin has no DynamicFlags/NPCFlags slot. C++ SetFlag/RemoveFlag
	// only mark the field dirty when the value changes (Object.cpp:804),
	// so the values-update broadcast fires on change only.
	methods["HasFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flag, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		var has bool
		switch field {
		case unitFieldFlags:
			has = state.UnitFlags&flag != 0
		case unitFieldFlags2:
			has = state.UnitFlags2&flag != 0
		case unitFieldPlayerFlags:
			has = state.PlayerFlags&flag != 0
		}
		return []any{has}, nil
	}
	methods["SetFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flag, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		var newVal uint32
		var changed bool
		switch field {
		case unitFieldFlags:
			newVal = state.UnitFlags | flag
			changed = newVal != state.UnitFlags
			state.UnitFlags = newVal
		case unitFieldFlags2:
			newVal = state.UnitFlags2 | flag
			changed = newVal != state.UnitFlags2
			state.UnitFlags2 = newVal
		case unitFieldPlayerFlags:
			newVal = state.PlayerFlags | flag
			changed = newVal != state.PlayerFlags
			state.PlayerFlags = newVal
		}
		if changed {
			s.sendPlayerValuesUpdate(map[int]uint32{int(field): newVal})
		}
		return nil, nil
	}
	methods["RemoveFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flag, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		var newVal uint32
		var changed bool
		switch field {
		case unitFieldFlags:
			newVal = state.UnitFlags &^ flag
			changed = newVal != state.UnitFlags
			state.UnitFlags = newVal
		case unitFieldFlags2:
			newVal = state.UnitFlags2 &^ flag
			changed = newVal != state.UnitFlags2
			state.UnitFlags2 = newVal
		case unitFieldPlayerFlags:
			newVal = state.PlayerFlags &^ flag
			changed = newVal != state.PlayerFlags
			state.PlayerFlags = newVal
		}
		if changed {
			s.sendPlayerValuesUpdate(map[int]uint32{int(field): newVal})
		}
		return nil, nil
	}
	// Get/Set*Value are Object::Get/Set*Value (ObjectMethods.h) over
	// the player slots backed by the session twin; see lua_values.go.
	registerIndexedValueMethods(methods, playerIndexedValueBackend(s, state))
	// IsInGroup is Player::GetGroup() != nullptr; the session group id
	// is the Go group membership (0 = ungrouped).
	methods["IsInGroup"] = luaNoArgs(func() any { return s.groupID != 0 })
	// GetRestBonus/SetRestBonus are Player::GetRestBonus/SetRestBonus
	// (PlayerMethods.h:1317-1321, 1810-1816); the setter runs through
	// setRestBonus, the same clamp + rest-state update as the XP path.
	methods["GetRestBonus"] = luaNoArgs(func() any { return state.RestBonus })
	// IsRested is Player::IsRested (PlayerMethods.h:633-637):
	// GetRestBonus() > 0.0f over the same stored bonus.
	methods["IsRested"] = luaNoArgs(func() any { return state.RestBonus > 0 })
	// GetDrunkValue/SetDrunkValue are Player::GetDrunkValue/
	// SetDrunkValue (PlayerMethods.h:1002-1006, 1692-1699;
	// Player.h:1809; Player.cpp:999-1037): the PLAYER_BYTES_3
	// inebriation byte, Go's DrunkenState. The setter clamps at 100
	// and broadcasts via sendPlayerUpdate, the .modify drunk twin;
	// the fake-inebriate aura max, the CrossedInebriationThreshold
	// packet, and the sobering-timer reset have no Go model.
	methods["GetDrunkValue"] = luaNoArgs(func() any { return state.DrunkenState })
	methods["SetDrunkValue"] = func(_ context.Context, args []any) ([]any, error) {
		value, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		level := uint8(value)
		if level > 100 {
			level = 100
		}
		state.DrunkenState = uint16(level)
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	methods["SetRestBonus"] = func(ctx context.Context, args []any) ([]any, error) {
		bonus, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.setRestBonus(state, bonus, nextLevelXPFor(s.server, ctx, state.Level))
		return []any{}, nil
	}
	// GetXPRestBonus is Player::GetXPRestBonus (Player.cpp:9584-9595):
	// the rested bonus for xp, capped at xp, consumed from the bonus.
	methods["GetXPRestBonus"] = func(ctx context.Context, args []any) ([]any, error) {
		xp, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		rested := uint32(state.RestBonus)
		if rested > xp {
			rested = xp
		}
		s.setRestBonus(state, state.RestBonus-float32(rested), nextLevelXPFor(s.server, ctx, state.Level))
		return []any{rested}, nil
	}
	// SetSheath is Player::SetSheath (Player.cpp:9655-9682) via the
	// shared opcode helper: virtual-item slots from the equipped weapons,
	// then the sheath byte; states at or past MAX_SHEATH_STATE are
	// dropped silently, matching the Eluna guard (PlayerMethods.h:1680).
	methods["SetSheath"] = func(ctx context.Context, args []any) ([]any, error) {
		sheathed, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if sheathed >= maxSheathState {
			return []any{}, nil
		}
		s.applySheathState(ctx, sheathed)
		return []any{}, nil
	}
	// StartTaxi is PlayerMethods::StartTaxi (PlayerMethods.h): the path-id
	// overload of Player::ActivateTaxiPathTo, flying a DBC or
	// AddTaxiPath-registered path without the flightmaster interact or
	// taximask gates of the packet handler.
	methods["StartTaxi"] = func(ctx context.Context, args []any) ([]any, error) {
		pathID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.startTaxiByPathID(ctx, pathID)
		return []any{}, nil
	}
	// Say/Yell/Whisper/TextEmote are PlayerMethods (PlayerMethods.h:
	// 3064-3128): Player::Say/Yell/Whisper/TextEmote (Player.cpp:
	// 20975-21072) over the session's broadcastChat fan-out — the same
	// range + emote two-side gates the client chat path uses — with the
	// OnPlayerChat hook fired first (cancel drops, rewrite wins, per
	// firePlayerChatHook). Whisper forces LANG_UNIVERSAL unless
	// LANG_ADDON and announces the target's AFK/DND reply; the
	// accept-whispers flip has no Go model (see commands.go) so it is
	// skipped, and an offline receiver drops silently instead of
	// tripping the C++ ASSERT. The SendUnit* twins are the UnitMethods
	// registrations Eluna puts on the Player metatable too
	// (LuaFunctions.cpp:1356-1357): unit->Say/Yell/Whisper/TextEmote
	// dispatch to the Player overrides, so they bind the same arms —
	// SendUnitWhisper's bossWhisper and SendUnitEmote's receiver and
	// bossEmote are accepted and ignored, matching the Player
	// overrides, which ignore them.
	playerSay := func(chatType uint8) scripting.ObjectMethod {
		return func(ctx context.Context, args []any) ([]any, error) {
			if s.server == nil {
				return []any{}, nil
			}
			message, err := luaStringArg(args, 0)
			if err != nil {
				return nil, err
			}
			language, err := luaUint32Arg(args, 1)
			if err != nil {
				return nil, err
			}
			cancelled, rewritten := s.firePlayerChatHook(ctx, uint32(chatType), language, message, "")
			if cancelled {
				return []any{}, nil
			}
			s.server.broadcastChat(s, nil, chatType, language, rewritten, "", 0)
			return []any{}, nil
		}
	}
	methods["Say"] = playerSay(chatSay)
	methods["SendUnitSay"] = playerSay(chatSay)
	methods["Yell"] = playerSay(chatYell)
	methods["SendUnitYell"] = playerSay(chatYell)
	playerEmote := func(ctx context.Context, args []any) ([]any, error) {
		if s.server == nil {
			return []any{}, nil
		}
		message, err := luaStringArg(args, 0)
		if err != nil {
			return nil, err
		}
		cancelled, rewritten := s.firePlayerChatHook(ctx, uint32(chatEmote), languageUniversal, message, "")
		if cancelled {
			return []any{}, nil
		}
		s.server.broadcastChat(s, nil, chatEmote, languageUniversal, rewritten, "", 0)
		return []any{}, nil
	}
	methods["TextEmote"] = playerEmote
	methods["SendUnitEmote"] = playerEmote
	playerWhisper := func(ctx context.Context, args []any) ([]any, error) {
		if s.server == nil {
			return []any{}, nil
		}
		message, err := luaStringArg(args, 0)
		if err != nil {
			return nil, err
		}
		language, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if len(args) < 3 {
			return nil, fmt.Errorf("argument 3 is required")
		}
		receiverGUID := luaObjectGUID(args[2])
		if receiverGUID == 0 {
			return nil, fmt.Errorf("argument 3 must be a player")
		}
		receiver := s.server.findSessionByGUID(receiverGUID)
		if receiver == nil || receiver.player == nil {
			return []any{}, nil
		}
		if language != languageAddon {
			language = languageUniversal
		}
		cancelled, rewritten := s.firePlayerChatHook(ctx, uint32(chatWhisper), language, message, receiver.player.Name)
		if cancelled {
			return []any{}, nil
		}
		s.server.broadcastChat(s, receiver, chatWhisper, language, rewritten, "", 0)
		if language != languageAddon {
			s.announceChatAutoReply(receiver)
		}
		return []any{}, nil
	}
	methods["Whisper"] = playerWhisper
	methods["SendUnitWhisper"] = playerWhisper
	methods["GetMaxHealth"] = luaNoArgs(func() any { return state.MaxHealth })
	methods["GetHealth"] = luaNoArgs(func() any { return state.Health })
	// GetArmor is Unit::GetArmor (Unit.h:860: GetResistance(SPELL_SCHOOL_NORMAL)):
	// the Player::UpdateArmor result (player_state.go), the same value the
	// damage fold reads for player targets (combat.go). Backs the faction
	// champions melee UpdateThreat armor leg.
	methods["GetArmor"] = luaNoArgs(func() any { return state.Armor })
	methods["GetHealthPct"] = luaNoArgs(func() any {
		if state.MaxHealth == 0 {
			return float32(0)
		}
		return float32(state.Health) * 100 / float32(state.MaxHealth)
	})
	methods["GetPower"] = func(_ context.Context, args []any) ([]any, error) {
		power, err := playerPowerIndex(args, state.Class)
		if err != nil {
			return nil, err
		}
		return []any{state.Powers[power]}, nil
	}
	methods["GetMaxPower"] = func(_ context.Context, args []any) ([]any, error) {
		power, err := playerPowerIndex(args, state.Class)
		if err != nil {
			return nil, err
		}
		return []any{state.MaxPowers[power]}, nil
	}
	methods["GetPowerPct"] = func(_ context.Context, args []any) ([]any, error) {
		power, err := playerPowerIndex(args, state.Class)
		if err != nil {
			return nil, err
		}
		if state.MaxPowers[power] == 0 {
			return []any{float32(0)}, nil
		}
		return []any{float32(state.Powers[power]) * 100 / float32(state.MaxPowers[power])}, nil
	}
	methods["GetGMRank"] = luaNoArgs(func() any { return s.security })
	methods["IsGM"] = luaNoArgs(func() any { return s.security > 0 })
	methods["IsInCombat"] = luaNoArgs(func() any { return s.attackTarget != 0 || state.UnitFlags&unitFlagInCombat != 0 })
	methods["IsAlive"] = luaNoArgs(func() any { return state.Health > 0 })
	methods["IsDead"] = luaNoArgs(func() any { return state.Health == 0 })
	// IsDying is UnitMethods::IsDying (UnitMethods.h:316-324): Unit::isDying
	// (Unit.h:1141) reads m_deathState == JUST_DIED. For players that state
	// spans death until spirit release (Player::setDeathState, Player.cpp:
	// 1385-1423, promotes only at :4779 on release): Go's just-died window
	// is Health == 0 with the ghost flag clear — buildPlayerRepop (death.go)
	// sets the ghost flag and Health 1 on release, and killPlayer zeroes
	// Health at death (the Spirit of Redemption completion zeroes it when the
	// deferred JUST_DIED cascade lands, matching its C++ timing).
	methods["IsDying"] = luaNoArgs(func() any { return state.Health == 0 && state.PlayerFlags&playerFlagGhost == 0 })
	// IsInBack(target[, arc]) is WorldObject::isInBack(target, arc) =
	// !HasInArc(2*pi - arc, target) (Object.cpp:1417-1420) on the player
	// surface, twin of the creature bridge (lua_creature_events.go). Eluna's
	// arc defaults to pi. The target is a Lua object with X/Y/GUID fields
	// (e.g. the creature) or raw x, y numbers. Called on the victim object
	// it ports victim-perspective gates exactly: victim:IsInBack(creature)
	// == C++ victim->isInBack(me); the creature-surface form
	// creature:IsInBack(victim) is the reverse (victim behind the creature).
	methods["IsInBack"] = func(_ context.Context, args []any) ([]any, error) {
		var tx, ty float32
		var targetGUID uint64
		if len(args) == 0 {
			return nil, fmt.Errorf("target is required")
		}
		if obj, ok := args[0].(*scripting.Object); ok && obj != nil {
			fx, found := objectFloat32Field(obj, "X")
			if !found {
				return nil, fmt.Errorf("target is required")
			}
			fy, found := objectFloat32Field(obj, "Y")
			if !found {
				return nil, fmt.Errorf("target is required")
			}
			tx, ty = fx, fy
			targetGUID, _ = objectUint64Field(obj, "GUID")
		} else if len(args) >= 2 {
			x, err := luaFloat32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			y, err := luaFloat32Arg(args, 1)
			if err != nil {
				return nil, err
			}
			tx, ty = x, y
		} else {
			return nil, fmt.Errorf("target is required")
		}
		if targetGUID != 0 && targetGUID == state.GUID {
			return []any{false}, nil // self is always in the frontal arc, never behind it
		}
		arc := math.Pi
		if len(args) > 1 {
			a, err := luaFloat32Arg(args, 1)
			if err != nil {
				return nil, err
			}
			arc = float64(a)
		}
		return []any{!hasInArc(state.Orientation, state.X, state.Y, tx, ty, 2*math.Pi-arc)}, nil
	}
	// IsTargetableForAttack is Unit::isTargetableForAttack (Unit.h:1077,
	// Unit.cpp) for players: alive, no NON_ATTACKABLE (0x2) / NOT_SELECTABLE
	// (0x02000000) unit flags, not a GM, and when checkFakeDeath (C++ default
	// true) no feign-death (UNIT_STATE_DIED; Go's unitFlag2FeignDeath twin,
	// set on hunter feign at spells.go:25872). UNIT_STATE_UNATTACKABLE has no
	// Go model, so that arm is vacuous here. Twin of the session creature
	// binding (lua_objects.go:646); zuljin's claw-rage chain calls it on
	// player targets, where the alive-only fallback missed feigning hunters.
	methods["IsTargetableForAttack"] = func(_ context.Context, args []any) ([]any, error) {
		checkFakeDeath := true
		if len(args) > 0 {
			if b, ok := args[0].(bool); ok {
				checkFakeDeath = b
			}
		}
		if state.Health == 0 {
			return []any{false}, nil
		}
		if state.UnitFlags&(unitFlagNonAttackable|0x02000000) != 0 {
			return []any{false}, nil
		}
		if s.security > 0 {
			return []any{false}, nil
		}
		if checkFakeDeath && state.UnitFlags2&unitFlag2FeignDeath != 0 {
			return []any{false}, nil
		}
		return []any{true}, nil
	}
	// IsFlying is Unit::IsFlying (Unit.h:1644): HasMovementFlag(FLYING |
	// DISABLE_GRAVITY) on the player's movement info. Go's twin is the
	// session's last client movement packet (lastMovementInfoSnapshot),
	// sanitized like C++ WorldSession validation (flying kept only with
	// a fly aura or GM security), so legitimate flyers keep the flag. A
	// player with no movement packet yet reads false, matching C++'s
	// zero-flag default. Taxi passengers read pre-boarding flags: Go
	// simulates taxi server-side with no spline model, so no in-flight
	// packet ever lands. Backs the air-force tripwire flying skip
	// (npcs_special.cpp:174).
	methods["IsFlying"] = luaNoArgs(func() any {
		if mi, ok := s.lastMovementInfoSnapshot(); ok {
			return mi.Flags&(movementFlying|movementDisableGravity) != 0
		}
		return false
	})
	// IsHostileTo(target) is WorldObject::IsHostileTo (Object.cpp:2842-2845)
	// from the player side, via playerHostileToUnit: the target's faction
	// template id resolves from a creature object (its motion's faction) or
	// a player object (its effective faction template); a raw GUID resolves
	// the same way. A creature the player owns (pet/minion) reads not
	// hostile, matching the same-charmer-or-owner friendly arm.
	methods["IsHostileTo"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 || s.server == nil {
			return []any{false}, nil
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 || targetGUID == s.playerGUID {
			return []any{false}, nil
		}
		if tm := s.server.findCreatureMotion(state.Map, state.InstanceID, targetGUID); tm != nil {
			if tm.OwnerGUID != 0 && tm.OwnerGUID == s.playerGUID {
				return []any{false}, nil
			}
			return []any{s.server.playerHostileToUnit(s, tm.Faction)}, nil
		}
		if ts := s.server.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
			return []any{s.server.playerHostileToUnit(s, ts.effectiveFactionTemplate(ts.player.Race))}, nil
		}
		return []any{false}, nil
	}
	methods["IsFullHealth"] = luaNoArgs(func() any { return state.MaxHealth > 0 && state.Health >= state.MaxHealth })
	methods["IsMounted"] = luaNoArgs(func() any { return state.MountDisplayID != 0 })
	methods["GetMountId"] = luaNoArgs(func() any { return state.MountDisplayID })
	methods["GetPetGUID"] = luaNoArgs(func() any { return state.PetGUID })
	// GetCharmerGUID is Unit::GetCharmerGUID (Unit.h:1158):
	// UNIT_FIELD_CHARMEDBY. Go stores no charmer for players (the
	// GetControllerGUID precedent), so this reads 0.
	methods["GetCharmerGUID"] = luaNoArgs(func() any { return uint64(0) })
	// GetCharmGUID is Unit::GetCharmedGUID (Unit.h:1161) via the
	// shared instance scan for a motion charmed by this player.
	methods["GetCharmGUID"] = luaNoArgs(func() any {
		return s.server.unitCharmedGUID(state.Map, state.InstanceID, state.GUID)
	})
	// GetCritterGUID is Unit::GetCritterGUID (Unit.h:1156):
	// UNIT_FIELD_CRITTER, the vanity-pet mirror on the player state.
	methods["GetCritterGUID"] = luaNoArgs(func() any { return state.CritterGUID })
	// SetCritterGUID is Unit::SetCritterGUID (UnitMethods.h:1908):
	// the UNIT_FIELD_CRITTER write; the values builder mirrors the
	// state field, so a player update propagates it.
	methods["SetCritterGUID"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		state.CritterGUID = guid
		s.sendPlayerUpdate()
		return nil, nil
	}
	// GetMinionGUID is Unit::GetMinionGUID (Unit.h:1151):
	// UNIT_FIELD_SUMMON. Go's player values builder writes the pet
	// GUID into that field, so this reads the pet slot. DELTA: a
	// guardian summoned before the pet sits earlier in C++'s
	// SUMMON list, where Go's single-slot model reads the pet.
	methods["GetMinionGUID"] = luaNoArgs(func() any { return state.PetGUID })
	// SetPetGUID is Unit::SetPetGUID (UnitMethods.h:1806): the
	// m_SummonSlot[PET] write, which Go models as the player PetGUID
	// (the values builder mirrors it into UNIT_FIELD_SUMMON).
	// LuaFunctions.cpp:331 registers SetMinionGUID to this same
	// C++ function, so this binding closes both names. No pet
	// object is created or dismissed here, matching the C++ slot
	// write; the creature leg stays out (no summon-slot model).
	methods["SetPetGUID"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		state.PetGUID = guid
		s.sendPlayerUpdate()
		return nil, nil
	}
	// HasSpellCooldown is PlayerMethods.h:217 (TRINITY arm): the
	// SpellHistory::HasCooldown twin in death.go — the per-spell entry,
	// then the DBC-category arm. The previous body compared the
	// seconds-based End against UnixMilli and so read false for every
	// live cooldown.
	methods["HasSpellCooldown"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{s.spellHasCooldown(spell)}, nil
	}
	// cooldownUpdate mirrors Eluna CHECKVAL<bool>(L, 3, true): an omitted
	// update argument means true, a non-boolean argument is an error.
	cooldownUpdate := func(method string, args []any) (bool, error) {
		if len(args) < 2 {
			return true, nil
		}
		update, ok := args[1].(bool)
		if !ok {
			return false, fmt.Errorf("%s requires a boolean update flag", method)
		}
		return update, nil
	}
	// GetSpellCooldownDelay is PlayerMethods.h:842 (TRINITY arm):
	// SpellHistory::GetRemainingCooldown in milliseconds, 0 for an
	// unknown spell (GetSpellInfo nil). The DBC-miss category is 0, so
	// the category arm stays silent without a DBC; a live spell entry
	// still answers, and entries only exist for DBC-known spells (the
	// DB load drops unknown rows), so the arms agree.
	methods["GetSpellCooldownDelay"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		var category uint32
		if s.server != nil && s.server.Data != nil {
			if spellInfo, found, spellErr := s.server.Data.Spell(spell); spellErr == nil && found {
				category = spellInfo.Category
			}
		}
		return []any{s.playerCooldownRemainingMs(spell, category)}, nil
	}
	// ResetSpellCooldown is PlayerMethods.h:3501 (TRINITY arm):
	// SpellHistory::ResetCooldown, silent when the spell holds no entry.
	methods["ResetSpellCooldown"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		update, err := cooldownUpdate("ResetSpellCooldown", args)
		if err != nil {
			return nil, err
		}
		s.resetPlayerCooldown(spell, update)
		return []any{}, nil
	}
	// ResetTypeCooldowns is PlayerMethods.h:3519 (TRINITY arm):
	// SpellHistory::ResetCooldowns over the predicate "spell's DBC
	// category == category", silent when nothing matches.
	methods["ResetTypeCooldowns"] = func(_ context.Context, args []any) ([]any, error) {
		category, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		update, err := cooldownUpdate("ResetTypeCooldowns", args)
		if err != nil {
			return nil, err
		}
		s.resetPlayerCooldownsByCategory(category, update)
		return []any{}, nil
	}
	// ResetAllCooldowns is PlayerMethods.h:3544 (TRINITY arm):
	// SpellHistory::ResetAllCooldowns — clears spell and category
	// cooldowns, touches neither school lockouts nor global cooldowns.
	// The Go list carries category ends on its entries, so clearing the
	// list clears both, matching clearPlayerCooldowns.
	methods["ResetAllCooldowns"] = func(_ context.Context, _ []any) ([]any, error) {
		s.clearPlayerCooldowns()
		return []any{}, nil
	}
	// IsInWater is Unit::IsInWater (Unit.cpp:3276-3279): the liquid
	// status IN_WATER|UNDER_WATER over all liquid types, not the client
	// swim flag — a wading player is in water without swimming, and a
	// swimming-flagged player over vmap-only water is not by the grid
	// leg. Same grid-leg limitation as IsUnderWater below.
	methods["IsInWater"] = luaNoArgs(func() any {
		if s == nil || s.player == nil || s.server == nil {
			return false
		}
		return s.server.mapIsInWater(s.player.Map, s.player.X, s.player.Y, s.player.Z)
	})
	// IsUnderWater is LuaUnit::IsUnderWater (LuaFunctions.cpp:352) via
	// Map::IsUnderWater (Map.cpp:2923-2926): the grid-leg liquid status
	// over water and ocean, under water at the default collision height.
	// The client swimming flag alone cannot answer it — swimming covers
	// wading — so this reads the server liquid model; a missing tile or
	// a vmap-only water volume reads false, the grid-leg limitation.
	methods["IsUnderWater"] = luaNoArgs(func() any {
		if s == nil || s.player == nil || s.server == nil {
			return false
		}
		return s.server.mapIsUnderWater(s.player.Map, s.player.X, s.player.Y, s.player.Z)
	})
	methods["IsMoving"] = luaNoArgs(func() any { return s.isMoving })
	methods["CanFly"] = luaNoArgs(func() any { return state.MountDisplayID != 0 })
	// CanParry/CanBlock are Unit::CanParry/CanBlock (Player.h:1819-1821):
	// the m_canParry/m_canBlock flags Go tracks on playerState and
	// already reads for the melee-outcome arms.
	methods["CanParry"] = luaNoArgs(func() any { return state.CanParry })
	methods["CanBlock"] = luaNoArgs(func() any { return state.CanBlock })
	// CanTitanGrip is Player::CanTitanGrip (Player.h:1823): the
	// m_canTitanGrip flag, set by the SPELL_EFFECT_TITAN_GRIP arm
	// (SpellEffects.cpp:5408) and cleared on talent reset
	// (Player.cpp:3887). Go keeps no such flag; the talent spell
	// (46917) known-check is the in-tree proxy the equip arms use.
	methods["CanTitanGrip"] = luaNoArgs(func() any { return playerHasSpell(state, 46917) })
	// CanShareQuest is Player::CanShareQuest (PlayerMethods.h:235-242;
	// Player.cpp:16145) via the quest_details.go helper: sharable flag
	// plus an active status row. The pool-inactive arm (and its
	// QUEST_PARTY_MSG_CANT_BE_SHARED_TODAY push) has no Go model.
	methods["CanShareQuest"] = func(ctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s.server == nil {
			return []any{false}, nil
		}
		return []any{s.canShareQuest(ctx, entry)}, nil
	}
	// CanUninviteFromGroup is Player::CanUninviteFromGroup
	// (PlayerMethods.h:263-269; Player.cpp:24420): the Eluna binding
	// passes no member, so the leader-target arm is vacuous. Non-LFG
	// groups need leader-or-assistant standing and no live
	// battleground; LFG groups read false — the kick-vote/kicks-left
	// legs have no Go model.
	methods["CanUninviteFromGroup"] = func(_ context.Context, _ []any) ([]any, error) {
		if s.groupID == 0 || s.server == nil {
			return []any{false}, nil
		}
		g := s.server.findGroupByID(s.groupID)
		if g == nil || g.IsLFG {
			return []any{false}, nil
		}
		if !g.isLeaderOrAssistant(s.playerGUID) {
			return []any{false}, nil
		}
		id, _ := s.luaBattlegroundID()
		return []any{id == 0}, nil
	}
	// CanUseItem is Player::CanUseItem (PlayerMethods.h:194-210): an
	// Item object takes the Item* arm (dead gate with the
	// not_loading=true default, bound-to-another-owner denial, then
	// the template arms), an entry takes the template arm (level gate
	// plus the same template arms, no dead gate). Both delegate to
	// canUseItemResult, the handleUseItem twin, so the AX1/AX2
	// skill-max side effects and the OnCanUseItem hook fire as in C++.
	methods["CanUseItem"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		var entry uint32
		isObject := false
		if obj, ok := args[0].(*scripting.Object); ok && obj != nil {
			isObject = true
			entryValue, ok := objectUint32Field(obj, "Entry")
			if !ok {
				return []any{false}, nil
			}
			entry = entryValue
			if guid, ok := objectUint64Field(obj, "GUID"); ok && guid != 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				var ownerGUID int64
				high := uint64(0x4000) << 48
				if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT owner_guid FROM item_instance WHERE guid = ? OR guid = ? LIMIT 1", guid, guid|high).Scan(&ownerGUID); err == nil && ownerGUID > 0 && uint64(ownerGUID) != s.playerGUID {
					return []any{false}, nil
				}
			}
		} else {
			value, err := luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			entry = value
		}
		if isObject && state.Health == 0 {
			return []any{false}, nil
		}
		data, err := s.loadItemQueryData(ctx, entry)
		if err != nil || data.Entry == 0 {
			return []any{false}, nil
		}
		if uint32(state.Level) < data.RequiredLevel {
			return []any{false}, nil
		}
		var spellIDs [5]int64
		for i := range spellIDs {
			spellIDs[i] = int64(data.Spells[i].ID)
		}
		result := s.canUseItemResult(ctx, entry, data.Class, data.SubClass, data.Flags2, int64(data.AllowableClass), int64(data.AllowableRace), data.RequiredSkill, data.RequiredSkillRank, data.RequiredSpell, data.HolidayID, data.Quality, data.RequiredReputationFaction, data.RequiredReputationRank, spellIDs)
		return []any{result == equipErrOk}, nil
	}
	methods["HasSpell"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		for _, known := range state.Spells {
			if known.ID == spell && known.Active && !known.Disabled {
				return []any{true}, nil
			}
		}
		return []any{false}, nil
	}
	methods["HasSkill"] = func(_ context.Context, args []any) ([]any, error) {
		skill, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		for _, known := range state.Skills {
			if uint32(known.Skill) == skill && known.Max > 0 {
				return []any{true}, nil
			}
		}
		return []any{false}, nil
	}
	// GetSkillValue is Player::GetSkillValue (Player.cpp:6240): the stored
	// value plus the temp and permanent bonuses, clamped at 0, and 0 when
	// the skill is absent. playerSkillTotalValue carries the fold: the
	// stored Bonus plus the live talent (98) aura bonus is the permanent
	// term, the live temp (30) aura bonus the temporary term.
	methods["GetSkillValue"] = func(_ context.Context, args []any) ([]any, error) {
		skill, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{uint32(s.playerSkillTotalValue(skill))}, nil
	}
	findPlayerSkill := func(skill uint32) int {
		if skill == 0 {
			return -1
		}
		for index, known := range state.Skills {
			if uint32(known.Skill) == skill {
				return index
			}
		}
		return -1
	}
	skillPermBonus := func(skill uint32) int32 {
		perm := int32(0)
		if index := findPlayerSkill(skill); index >= 0 {
			perm = int32(int16(state.Skills[index].Bonus))
		}
		_, talent := s.skillAuraBonus(skill)
		return perm + talent
	}
	skillTempBonus := func(skill uint32) int32 {
		temp, _ := s.skillAuraBonus(skill)
		return temp
	}
	persistPlayerSkill := func(ctx context.Context, sk playerSkill) {
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return
		}
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, ?, ?)", state.GUID, sk.Skill, sk.Value, sk.Max)
	}
	removePlayerSkillRow := func(ctx context.Context, skill uint32) {
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return
		}
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_skills WHERE guid = ? AND skill = ?", state.GUID, skill)
	}
	advancePlayerSkill := func(ctx context.Context, index int, step uint32) {
		known := &state.Skills[index]
		if known.Max == 0 || known.Value == 0 || known.Value >= known.Max {
			return
		}
		if newVal := uint32(known.Value) + step; newVal > uint32(known.Max) {
			known.Value = known.Max
		} else {
			known.Value = uint16(newVal)
		}
		persistPlayerSkill(ctx, *known)
		s.setAchievementCriteria(criteriaTypeReachSkillLevel, uint32(known.Skill), uint32(known.Value))
	}
	// GetPureSkillValue is Player::GetPureSkillValue (Player.cpp:6300):
	// the stored value with no bonus fold, 0 for skill 0 or an absent skill.
	methods["GetPureSkillValue"] = func(_ context.Context, args []any) ([]any, error) {
		skill, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if index := findPlayerSkill(skill); index >= 0 {
			return []any{uint32(state.Skills[index].Value)}, nil
		}
		return []any{uint32(0)}, nil
	}
	// GetBaseSkillValue is Player::GetBaseSkillValue (Player.cpp:6286): the
	// stored value plus the permanent bonus only, clamped at 0.
	methods["GetBaseSkillValue"] = func(_ context.Context, args []any) ([]any, error) {
		skill, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if index := findPlayerSkill(skill); index >= 0 {
			if total := int32(state.Skills[index].Value) + skillPermBonus(skill); total > 0 {
				return []any{uint32(total)}, nil
			}
		}
		return []any{uint32(0)}, nil
	}
	// GetMaxSkillValue is Player::GetMaxSkillValue (Player.cpp:6257): the
	// stored max plus both bonuses, clamped at 0.
	methods["GetMaxSkillValue"] = func(_ context.Context, args []any) ([]any, error) {
		skill, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if index := findPlayerSkill(skill); index >= 0 {
			if total := int32(state.Skills[index].Max) + skillTempBonus(skill) + skillPermBonus(skill); total > 0 {
				return []any{uint32(total)}, nil
			}
		}
		return []any{uint32(0)}, nil
	}
	// GetPureMaxSkillValue is Player::GetPureMaxSkillValue (Player.cpp:6274):
	// the stored max with no bonus fold.
	methods["GetPureMaxSkillValue"] = func(_ context.Context, args []any) ([]any, error) {
		skill, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if index := findPlayerSkill(skill); index >= 0 {
			return []any{uint32(state.Skills[index].Max)}, nil
		}
		return []any{uint32(0)}, nil
	}
	// GetSkillTempBonusValue is Player::GetSkillTempBonusValue
	// (Player.cpp:6324): the live temp (30) aura bonus, 0 when absent.
	methods["GetSkillTempBonusValue"] = func(_ context.Context, args []any) ([]any, error) {
		skill, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if findPlayerSkill(skill) < 0 {
			return []any{int32(0)}, nil
		}
		return []any{skillTempBonus(skill)}, nil
	}
	// GetSkillPermBonusValue is Player::GetSkillPermBonusValue
	// (Player.cpp:6312): the stored permanent bonus plus the live talent
	// (98) aura bonus, 0 when absent.
	methods["GetSkillPermBonusValue"] = func(_ context.Context, args []any) ([]any, error) {
		skill, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if findPlayerSkill(skill) < 0 {
			return []any{int32(0)}, nil
		}
		return []any{skillPermBonus(skill)}, nil
	}
	// SetSkill is Player::SetSkill (Player.cpp:6114) under the TRINITY arg
	// order (id, step, currVal, maxVal) of PlayerMethods.h:1718. A zero id
	// is a no-op; a zero currVal on a known skill removes it (row deleted,
	// live bonus maps cleared, first-rank spells unlearned via the
	// SkillLineAbility cascade); a nonzero currVal on an unknown skill
	// adds it when a SkillLine row exists and a slot is free, re-applying
	// live skill bonuses the way the C++ add arm does. The
	// UpdateSkillEnchantments leg has no Go model.
	methods["SetSkill"] = func(ctx context.Context, args []any) ([]any, error) {
		skillArg := func(index int) (uint16, error) {
			value, err := luaUint32Arg(args, index)
			if err != nil {
				return 0, err
			}
			if value > math.MaxUint16 {
				return 0, fmt.Errorf("argument %d must be uint16", index+1)
			}
			return uint16(value), nil
		}
		id, err := skillArg(0)
		if err != nil {
			return nil, err
		}
		step, err := skillArg(1)
		if err != nil {
			return nil, err
		}
		currVal, err := skillArg(2)
		if err != nil {
			return nil, err
		}
		maxVal, err := skillArg(3)
		if err != nil {
			return nil, err
		}
		if id == 0 {
			return []any{}, nil
		}
		if index := findPlayerSkill(uint32(id)); index >= 0 {
			if currVal == 0 {
				state.Skills = append(state.Skills[:index], state.Skills[index+1:]...)
				delete(s.skillTempBonus, uint32(id))
				delete(s.skillTalentBonus, uint32(id))
				removePlayerSkillRow(ctx, uint32(id))
				if s.server != nil && s.server.Data != nil {
					if abilities, ok, aerr := s.server.Data.SkillLineAbilitiesForSkill(uint32(id)); aerr == nil && ok {
						visited := make(map[uint32]bool)
						for _, ab := range abilities {
							s.removeSpell(ctx, s.server.getFirstSpellInChain(ab.Spell), true, visited)
						}
					}
				}
				return []any{}, nil
			}
			state.Skills[index].Step = step
			state.Skills[index].Value = currVal
			state.Skills[index].Max = maxVal
			persistPlayerSkill(ctx, state.Skills[index])
			s.learnSkillRewardedSpells(ctx, uint32(id), uint32(currVal))
			s.setAchievementCriteria(criteriaTypeReachSkillLevel, uint32(id), uint32(currVal))
			s.setAchievementCriteria(criteriaTypeLearnSkillLevel, uint32(id), uint32(maxVal))
			return []any{}, nil
		}
		if currVal == 0 {
			return []any{}, nil
		}
		if s.server != nil && s.server.Data != nil {
			if _, found, err := s.server.Data.SkillLineCategory(uint32(id)); err == nil && !found {
				return []any{}, nil
			}
		}
		if len(state.Skills) >= playerMaxSkills {
			return []any{}, nil
		}
		state.Skills = append(state.Skills, playerSkill{Skill: id, Step: step, Value: currVal, Max: maxVal})
		persistPlayerSkill(ctx, state.Skills[len(state.Skills)-1])
		s.reapplySkillBonusAuras(uint32(id))
		s.learnSkillRewardedSpells(ctx, uint32(id), uint32(currVal))
		s.setAchievementCriteria(criteriaTypeReachSkillLevel, uint32(id), uint32(currVal))
		s.setAchievementCriteria(criteriaTypeLearnSkillLevel, uint32(id), uint32(maxVal))
		return []any{}, nil
	}
	// AdvanceSkill is PlayerMethods.h:3354: a zero id or step is a no-op,
	// and an unknown skill is a no-op, matching Player::HasSkill's
	// presence gate (not the Go HasSkill Max>0 quirk). The advance itself
	// is Player::UpdateSkill (Player.cpp:5743), capped at the max; the
	// enchantment leg is unmodeled.
	methods["AdvanceSkill"] = func(ctx context.Context, args []any) ([]any, error) {
		skill, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		step, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if skill == 0 || step == 0 {
			return []any{}, nil
		}
		if index := findPlayerSkill(skill); index >= 0 {
			advancePlayerSkill(ctx, index, step)
		}
		return []any{}, nil
	}
	// AdvanceAllSkills is PlayerMethods.h:3319: a zero step is a no-op;
	// every known skill advances by UpdateSkill except language (10) and
	// generic (12) SkillLine categories (SharedDefines.h:3084/3086).
	// Without a DBC the language range type is the fallback skip.
	methods["AdvanceAllSkills"] = func(ctx context.Context, args []any) ([]any, error) {
		step, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if step == 0 {
			return []any{}, nil
		}
		for index := range state.Skills {
			skill := state.Skills[index].Skill
			if s.server != nil && s.server.Data != nil {
				if cat, found, err := s.server.Data.SkillLineCategory(uint32(skill)); err == nil && found && (cat == 10 || cat == 12) {
					continue
				}
			} else if s.skillRangeType(state.Race, state.Class, skill) == wotlk.SkillRangeLanguage {
				continue
			}
			advancePlayerSkill(ctx, index, step)
		}
		return []any{}, nil
	}
	// AdvanceSkillsToMax is PlayerMethods.h:3306 under TRINITY:
	// Player::UpdateWeaponsSkillsToMaxSkillsForLevel (Player.cpp:6087)
	// sets every non-profession, non-riding skill with max > 1 to its max.
	// Profession is the SkillLine category 11 plus fishing, cooking and
	// first aid (SpellMgr.h:562-570); without a DBC the primary-profession
	// ids are the fallback. The defense UpdateDefenseBonusesMod leg has
	// no Go defense-bonus model.
	methods["AdvanceSkillsToMax"] = func(ctx context.Context, args []any) ([]any, error) {
		isProfessionOrRiding := func(skill uint32) bool {
			if skill == 762 || skill == 356 || skill == 185 || skill == 129 {
				return true
			}
			if s.server != nil && s.server.Data != nil {
				if cat, found, err := s.server.Data.SkillLineCategory(skill); err == nil && found {
					return cat == skillLineCategoryProfession
				}
			}
			switch skill {
			case 171, 164, 333, 202, 182, 773, 755, 165, 186, 393, 197:
				return true
			}
			return false
		}
		for index := range state.Skills {
			known := &state.Skills[index]
			if isProfessionOrRiding(uint32(known.Skill)) {
				continue
			}
			if known.Max > 1 {
				known.Value = known.Max
				persistPlayerSkill(ctx, *known)
			}
		}
		return []any{}, nil
	}
	itemCount := func(args []any) ([]any, error) {
		item, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		count := int64(0)
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return []any{uint32(0)}, nil
		}
		err = s.server.CharactersStore.DB.QueryRow("SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ?", state.GUID, item).Scan(&count)
		if isMissingTableError(err) {
			return []any{uint32(0)}, nil
		}
		if count < 0 {
			count = 0
		}
		return []any{uint32(count)}, err
	}
	methods["GetItemCount"] = func(_ context.Context, args []any) ([]any, error) { return itemCount(args) }
	methods["HasItem"] = func(_ context.Context, args []any) ([]any, error) {
		values, err := itemCount(args)
		if err != nil {
			return nil, err
		}
		required := uint32(1)
		if len(args) > 1 {
			required, err = luaUint32Arg(args, 1)
			if err != nil {
				return nil, err
			}
		}
		count := values[0].(uint32)
		return []any{count >= required}, nil
	}
	// DestroyItemCount(entry, count[, update]) is the
	// Player::DestroyItemCount twin (Player.cpp:12727): it walks the
	// player's stacks of the entry in bag/slot order, deleting whole
	// stacks and trimming the stack that covers the remainder, and
	// returns the total actually destroyed. C++'s partial-stack branch
	// returns only the full-stack subtotal; this bridge returns the
	// full destroyed total instead (the rows left behind are the same).
	// The update flag mirrors C++'s send-update leg: a live inventory
	// refresh is pushed only when it is true. The C++ in-trade skip
	// has no Go row model and is not replicated.
	methods["DestroyItemCount"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("DestroyItemCount requires an item entry and a count")
		}
		item, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		count, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		update := true
		if len(args) > 2 {
			if b, ok := args[2].(bool); ok {
				update = b
			}
		}
		destroyed, err := s.destroyItemEntryCount(ctx, item, count, update)
		if err != nil {
			return nil, err
		}
		return []any{destroyed}, nil
	}
	// AddItem(entry[, count]) is Player::AddItem (PlayerMethods.h:3424-3458):
	// CanStoreNewItem (NULL_BAG/NULL_SLOT, partial-count reduction) then
	// StoreNewItem + SendNewItem, returning the stored item or nil. Go
	// stores through storeOrStackItem (the .additem twin): an unknown
	// template reads nil, and a full inventory reads nil — Go stores
	// all-or-nothing where C++ stores the fitting part (the .additem
	// command shares this delta). New stacks carry randomPropertyId 0
	// where C++ rolls GenerateItemRandomPropertyId. The quest-item
	// adjust fires inside storeOrStackItem; the SMSG_ITEM_PUSH_RESULT
	// below is the SendNewItem twin, followed by the usual refresh.
	methods["AddItem"] = func(ctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		count := uint32(1)
		if len(args) > 1 {
			if c, err := luaUint32Arg(args, 1); err != nil {
				return nil, err
			} else {
				count = c
			}
		}
		if count == 0 {
			return []any{nil}, nil
		}
		if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return []any{nil}, nil
		}
		var exists uint32
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT entry FROM item_template WHERE entry = ? LIMIT 1", entry).Scan(&exists); err != nil || exists == 0 {
			return []any{nil}, nil
		}
		res, err := s.storeOrStackItem(ctx, state.GUID, entry, count)
		if err != nil || res == nil || res.ItemGUID == 0 {
			return []any{nil}, nil
		}
		slotForPush := uint32(res.Slot)
		if res.IsStack {
			slotForPush = 0xFFFFFFFF
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildItemPushResult(s.playerGUID, res.ClientBag, slotForPush, entry, count, res.InventoryCount, res.IsStack), true)
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
		if obj := s.sessionLuaItem(ctx, res.ItemGUID); obj != nil {
			return []any{obj}, nil
		}
		return []any{nil}, nil
	}
	// RemoveItem(item|entry, count) is Player::RemoveItem
	// (PlayerMethods.h:3461-3481): the entry arm is
	// DestroyItemCount(entry, count, true), the item arm is
	// DestroyItemCount(item, count, true) with the Eluna-object
	// invalidation when the whole stack goes. Go has no object
	// invalidation on the scripting surface, so a destroyed item's
	// object re-reads its row and finds nothing. The entry arm reuses
	// destroyItemEntryCount, the shared DestroyItemCount walk; the item
	// arm mirrors its full/partial split for the single stack. An item
	// owned by another player, a missing row, or a count of 0 is a
	// silent no-op.
	methods["RemoveItem"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		count, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if obj, ok := args[0].(*scripting.Object); ok && obj != nil {
			if _, hasEntry := objectUint32Field(obj, "Entry"); !hasEntry {
				return nil, fmt.Errorf("argument 1 must be an item")
			}
			guid, _ := objectUint64Field(obj, "GUID")
			if guid == 0 || count == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
				return nil, nil
			}
			db := s.server.CharactersStore.DB
			high := uint64(0x4000) << 48
			var entry, instCount uint32
			var ownerGUID int64
			if err := db.QueryRowContext(ctx, "SELECT itemEntry, `count`, owner_guid FROM item_instance WHERE guid = ? OR guid = ? LIMIT 1", guid, guid|high).Scan(&entry, &instCount, &ownerGUID); err != nil || entry == 0 || ownerGUID <= 0 || uint64(ownerGUID) != s.playerGUID {
				return nil, nil
			}
			if instCount <= count {
				s.fireItemRemoveHook(ctx, guid)
				_, _ = db.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND (item = ? OR item = ?)", s.playerGUID, guid, guid|high)
				_, _ = db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ? OR guid = ?", guid, guid|high)
				s.adjustQuestItemCount(ctx, entry, instCount, false)
				s.applyItemObtainSpells(ctx, entry, 0, false)
				s.despawnItem(guid)
			} else {
				_, _ = db.ExecContext(ctx, "UPDATE item_instance SET `count` = `count` - ? WHERE guid = ? OR guid = ?", count, guid, guid|high)
				s.adjustQuestItemCount(ctx, entry, count, false)
			}
			_ = s.sendInventoryItems(ctx)
			s.sendPlayerUpdate()
			return nil, nil
		}
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		_, err = s.destroyItemEntryCount(ctx, entry, count, true)
		return nil, err
	}
	// EquipItem(item|entry, slot) is Player::EquipItem (PlayerMethods.h:3204-3243):
	// slots >= INVENTORY_SLOT_BAG_END (23) read nil; the entry arm
	// creates a fresh item (Item::CreateItem, count 1), runs
	// CanEquipItem(slot, dest, item, swap=false) — whose not_loading
	// default is true (Player.h:1091), so the stunned/combat/casting
	// block never runs on this path — then EquipItem(dest) plus
	// AutoUnequipOffhandIfNeed, and (entry arm only)
	// ItemAddedQuestCheck and the receive-epic criteria. The item arm
	// removes the item from its bag first. Go models the CanEquipItem
	// legs it has: slot fit (itemFitsEquipmentSlot, with its
	// dual-wield/titan-grip/offhand-2H legs; bag slots via
	// isSlotValidForItem), the CanUseItem template arms (CanEquipItem
	// funnels through CanUseItem), bind-owner and loot-generated
	// denials, occupied-dest denial (swap=false), the source unequip
	// legs, and the 2H-mainhand offhand move. Unmodeled, shared with
	// the deferred CanEquipItem model: unique-equipped,
	// CanTakeMoreSimilarItems, quiver-only-one, scaling-stat caps,
	// and item-set/_ApplyItemMods stat application (Go's equip funnel
	// applies none of these either). A failed equip returns nil; the
	// entry arm stages its fresh item in a free backpack slot first,
	// so a full backpack fails it where C++ equips straight into the
	// empty dest.
	methods["EquipItem"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		slotValue, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if slotValue >= uint32(invSlotBagEnd) {
			return []any{nil}, nil
		}
		slot := uint8(slotValue)
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			return []any{nil}, nil
		}
		db := s.server.CharactersStore.DB
		high := uint64(0x4000) << 48
		var itemGUID uint64
		var entry uint32
		entryArm := false
		var srcBagKey, srcSlot int64
		srcHasRow := false
		if obj, ok := args[0].(*scripting.Object); ok && obj != nil {
			if _, hasEntry := objectUint32Field(obj, "Entry"); !hasEntry {
				return nil, fmt.Errorf("argument 1 must be an item")
			}
			guid, _ := objectUint64Field(obj, "GUID")
			if guid == 0 {
				return []any{nil}, nil
			}
			itemGUID = guid
			var instCount uint32
			var ownerGUID int64
			if err := db.QueryRowContext(ctx, "SELECT itemEntry, `count`, owner_guid FROM item_instance WHERE guid = ? OR guid = ? LIMIT 1", guid, guid|high).Scan(&entry, &instCount, &ownerGUID); err != nil || entry == 0 || ownerGUID <= 0 || uint64(ownerGUID) != s.playerGUID {
				return []any{nil}, nil
			}
			_ = instCount
			if s.server.containerLootGenerated(guid) {
				return []any{nil}, nil
			}
			if err := db.QueryRowContext(ctx, "SELECT bag, slot FROM character_inventory WHERE guid = ? AND (item = ? OR item = ?) LIMIT 1", s.playerGUID, guid, guid|high).Scan(&srcBagKey, &srcSlot); err != nil {
				return []any{nil}, nil
			}
			srcHasRow = true
			if srcBagKey == 0 && srcSlot == int64(slot) {
				return []any{nil}, nil
			}
		} else {
			entry, err = luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			entryArm = true
		}
		data, err := s.loadItemQueryData(ctx, entry)
		if err != nil || data.Entry == 0 {
			return []any{nil}, nil
		}
		fits := false
		if slot >= uint8(invSlotBagStart) {
			fits = s.isSlotValidForItem(uint8(data.InventoryType), slot)
		} else {
			var equipped map[int64]itemQueryData
			if slot == uint8(equipSlotOffhand) {
				var mhItem int64
				if err := db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = 15 AND item != 0 LIMIT 1", s.playerGUID).Scan(&mhItem); err == nil && mhItem != 0 {
					var mhEntry int64
					if err := db.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", mhItem).Scan(&mhEntry); err == nil && mhEntry != 0 {
						if mhData, err := s.loadItemQueryData(ctx, uint32(mhEntry)); err == nil && mhData.Entry != 0 {
							equipped = map[int64]itemQueryData{15: mhData}
						}
					}
				}
			}
			fits = itemFitsEquipmentSlot(state, equipped, data, int64(slot))
		}
		if !fits {
			return []any{nil}, nil
		}
		if uint32(state.Level) < data.RequiredLevel {
			return []any{nil}, nil
		}
		var spellIDs [5]int64
		for i := range spellIDs {
			spellIDs[i] = int64(data.Spells[i].ID)
		}
		if result := s.canUseItemResult(ctx, entry, data.Class, data.SubClass, data.Flags2, int64(data.AllowableClass), int64(data.AllowableRace), data.RequiredSkill, data.RequiredSkillRank, data.RequiredSpell, data.HolidayID, data.Quality, data.RequiredReputationFaction, data.RequiredReputationRank, spellIDs); result != equipErrOk {
			return []any{nil}, nil
		}
		var dstItemGUID int64
		_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? AND item != 0 LIMIT 1", s.playerGUID, slot).Scan(&dstItemGUID)
		if dstItemGUID != 0 {
			return []any{nil}, nil
		}
		if srcHasRow && srcBagKey == 0 && (srcSlot < int64(equipSlotEnd) || (srcSlot >= int64(invSlotBagStart) && srcSlot < int64(invSlotBagEnd)) || (srcSlot >= 67 && srcSlot <= 73)) {
			if msg := s.canUnequipLegs(ctx, int64(itemGUID), int64(entry)); msg != equipErrOk {
				return []any{nil}, nil
			}
			if !(slot >= uint8(invSlotBagStart) && slot < uint8(invSlotBagEnd)) && s.itemIsNonemptyBag(ctx, itemGUID) {
				return []any{nil}, nil
			}
		}
		var offhandGUID int64
		offhandMove := false
		if slot == uint8(equipSlotMainhand) && data.InventoryType == 17 {
			_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? AND item != 0 LIMIT 1", s.playerGUID, equipSlotOffhand).Scan(&offhandGUID)
			offhandMove = offhandGUID != 0
		}
		if entryArm {
			stageSlot, ok := s.findFreeBackpackSlot(ctx)
			if !ok {
				return []any{nil}, nil
			}
			var maxDurability int64 = 100
			if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MaxDurability, 0) FROM item_template WHERE entry = ?", entry).Scan(&maxDurability); err != nil || maxDurability <= 0 {
				maxDurability = 100
			}
			newDuration := s.server.itemTemplateDuration(ctx, entry)
			var baseGUID int64
			if reserved, ok := s.server.generateItemGUIDRange(1); ok {
				baseGUID = int64(reserved)
			} else {
				_ = db.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) FROM item_instance").Scan(&baseGUID)
			}
			if baseGUID < 0 {
				baseGUID = 0
			}
			itemGUID = uint64(baseGUID) + 1
			if _, err := db.ExecContext(ctx, `INSERT INTO item_instance
				(guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text)
				VALUES (?, ?, ?, 0, ?, ?, '', 0, '', 0, ?, 0, '')`,
				itemGUID, entry, s.playerGUID, 1, newDuration, maxDurability); err != nil {
				return []any{nil}, nil
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", s.playerGUID, 0, stageSlot, itemGUID); err != nil {
				_, _ = db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
				return []any{nil}, nil
			}
			srcBagKey, srcSlot = 0, int64(stageSlot)
		}
		if offhandMove {
			freeSlot, ok := s.findFreeBackpackSlot(ctx)
			if !ok {
				if entryArm {
					_, _ = db.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, itemGUID)
					_, _ = db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
				}
				return []any{nil}, nil
			}
			_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = 0, slot = ? WHERE guid = ? AND item = ?", freeSlot, s.playerGUID, offhandGUID)
		}
		if entryArm {
			s.adjustQuestItemCount(ctx, entry, 1, true)
			s.updateAchievementCriteria(criteriaTypeReceiveEpicItem, entry, 1)
		}
		s.swapInventoryCoordinates(ctx, int64(itemGUID), srcBagKey, srcSlot, 0, 0, int64(slot))
		s.syncEquipmentCache(ctx)
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
		s.updateAchievementCriteria(criteriaTypeEquipItem, entry, 1)
		if data.Quality >= 5 {
			s.updateAchievementCriteria(criteriaTypeEquipEpicItem, uint32(slot), 1)
		}
		s.fireEquipHook(ctx, int64(itemGUID), 255, slot)
		s.sendEquippedItemTimeUpdate(ctx, int64(itemGUID))
		if obj := s.sessionLuaItem(ctx, itemGUID); obj != nil {
			return []any{obj}, nil
		}
		return []any{nil}, nil
	}
	// CanEquipItem(item|entry, slot) is Player::CanEquipItem
	// (PlayerMethods.h:3254-3289): slots >= EQUIPMENT_SLOT_END (19) read
	// false before any lookup, like the Eluna early return. The entry arm
	// runs the CanEquipNewItem feasibility core (canEquipItemEntry, shared
	// with the vendor buy path); the item arm resolves the entry from the
	// owned item_instance row and runs the same core with the item itself
	// excluded from the max-count, occupied-slot and unique-equipped legs,
	// plus the EquipItem ownership and loot-generated denials. The
	// not_loading in-game arms and the SSD/ItemLimitCategory/socketed-gem
	// terms stay unmodeled, shared with the deferred CanEquipItem model
	// the EquipItem comment names; a missing store or unowned item reads
	// false, never an error, except a non-item object argument.
	methods["CanEquipItem"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		slotValue, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if slotValue >= uint32(equipmentSlotEnd) {
			return []any{false}, nil
		}
		var entry uint32
		var excludeGUID uint64
		if obj, ok := args[0].(*scripting.Object); ok && obj != nil {
			if _, hasEntry := objectUint32Field(obj, "Entry"); !hasEntry {
				return nil, fmt.Errorf("argument 1 must be an item")
			}
			guid, _ := objectUint64Field(obj, "GUID")
			if guid == 0 {
				return []any{false}, nil
			}
			if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
				return []any{false}, nil
			}
			db := s.server.CharactersStore.DB
			high := uint64(0x4000) << 48
			var ownerGUID int64
			if err := db.QueryRowContext(ctx, "SELECT itemEntry, owner_guid FROM item_instance WHERE guid = ? OR guid = ? LIMIT 1", guid, guid|high).Scan(&entry, &ownerGUID); err != nil || entry == 0 || ownerGUID <= 0 || uint64(ownerGUID) != s.playerGUID {
				return []any{false}, nil
			}
			if s.server.containerLootGenerated(guid) {
				return []any{false}, nil
			}
			excludeGUID = guid
		} else {
			entry, err = luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
		}
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			return []any{false}, nil
		}
		return []any{s.canEquipItemEntry(ctx, entry, uint8(slotValue), excludeGUID) == equipErrOk}, nil
	}
	methods["HasAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		_, ok := s.auras[spell]
		return []any{ok}, nil
	}
	// GetAura is Unit::GetAura (UnitMethods.h, Unit.cpp GetAura(spellID)):
	// the engine record for the spell, or nil when absent. Presence-only
	// auras (s.auras without an activeAuras record) have no Aura object;
	// see lua_aura.go for the object surface and its deltas.
	methods["GetAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s.activeAuras[spell] == nil {
			return []any{nil}, nil
		}
		ref := &luaAuraRef{server: s.server, spellID: spell, player: s, mapID: state.Map, instanceID: state.InstanceID}
		return []any{ref.luaAuraObject()}, nil
	}
	// IsImmunedToDamage is the mask overload of Unit::IsImmunedToDamage
	// (Unit.cpp:7800-7816) for players: the session isImmuneToDamage twin
	// (Divine Shield / Ice Block / Blessing of Protection hardcodes plus
	// the live aura-39/aura-40 misc folds). SPELL_SCHOOL_MASK_NORMAL = 1.
	methods["IsImmunedToDamage"] = func(_ context.Context, args []any) ([]any, error) {
		mask, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{s.isImmuneToDamage(mask)}, nil
	}
	// IsVehicle is Unit::IsVehicle (Unit.h:842) for players: the
	// UNIT_MASK_VEHICLE bit, set in CreateVehicleKit and cleared in
	// RemoveVehicleKit (Unit.cpp:12047-12070) — a live player vehicle
	// kit is the exact twin. A passenger has no own kit (the kit sits
	// on the vehicle creature), so passengers read false, as in C++.
	methods["IsVehicle"] = func(_ context.Context, _ []any) ([]any, error) {
		return []any{s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.playerGUID) != nil}, nil
	}
	// GetVehicleKit is Unit::GetVehicleKit (UnitMethods.h, registered in
	// LuaFunctions.cpp): the Vehicle object for the unit's own kit, or
	// nil when the unit is not a vehicle base. The GetVehicle twin is
	// commented out in LuaFunctions.cpp, so it stays out.
	methods["GetVehicleKit"] = func(_ context.Context, _ []any) ([]any, error) {
		if s.server == nil {
			return []any{nil}, nil
		}
		return []any{s.server.luaVehicleObject(s.player.Map, s.player.InstanceID, s.playerGUID)}, nil
	}
	// IsOnVehicle is Unit::IsOnVehicle (UnitMethods.h): despite the doc
	// claiming bool, the Trinity arm pushes GetVehicle(), the Vehicle
	// object the unit rides (nil when not riding), so the Go arm pushes
	// the vehicle object too and Lua truthiness matches.
	methods["IsOnVehicle"] = func(_ context.Context, _ []any) ([]any, error) {
		if s.server == nil || s.player.VehicleGUID == 0 {
			return []any{nil}, nil
		}
		return []any{s.server.luaVehicleObject(s.player.Map, s.player.InstanceID, s.player.VehicleGUID)}, nil
	}
	methods["AddAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.applyAura(spell)
		return nil, nil
	}
	// CastSpell(target, spellId[, triggered]) is Unit::CastSpell on the
	// player caster: the session cast path (castSpellDirect, the
	// triggered pipeline — TRIGGERED_FULL_MASK, SpellDefines.h:155), the
	// player twin of the creature CastSpell bridge. A nil/unknown target
	// falls back to the caster itself (the terestian/akilzon self-cast
	// arms); a non-triggered flag still rides the session path, whose
	// power/cast-time legs are unmodeled exactly like the rest of Go's
	// fire-and-forget cast model. SPELL_EFFECT_SUMMON (28) has no
	// session-path executor yet, so summon-effect casts fire the
	// SMSG_SPELL_GO visual but spawn nothing.
	methods["CastSpell"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("CastSpell requires a target and a spell id")
		}
		spellID, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			targetGUID = state.GUID
		}
		s.castSpellDirect(ctx, spellID, targetGUID)
		return nil, nil
	}
	// SendCastResult(spellID, result) is the Spell::SendCastResult twin
	// (Spell.cpp): SMSG_CAST_FAILED with castID 0 for this player, the
	// spell id, and the result's extended payload. Backs the soulwell
	// already-has-stone arm (go_scripts.cpp:553-556,
	// SPELL_FAILED_TOO_MANY_OF_ITEM = 129, SharedDefines.h:1111).
	methods["SendCastResult"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("SendCastResult requires a spell id and a result")
		}
		spellID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		result, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.sendCastResult(ctx, spellID, uint8(result))
		return nil, nil
	}
	// GetSpellEffectItemType(spellID, effectIndex) is the
	// sSpellMgr->GetSpellInfo(spell)->Effects[i].ItemType read: the DBC
	// Spell.dbc effect row's created-item entry, 0 when the spell or the
	// effect index is missing. Backs the soulwell _stoneId derivation
	// (go_scripts.cpp:534, EFFECT_0).
	methods["GetSpellEffectItemType"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("GetSpellEffectItemType requires a spell id and an effect index")
		}
		spellID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		index, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if s.server == nil || s.server.Data == nil {
			return []any{uint32(0)}, nil
		}
		spell, ok, err := s.server.Data.Spell(spellID)
		if err != nil || !ok || int(index) >= len(spell.Effects) {
			return []any{uint32(0)}, nil
		}
		return []any{spell.Effects[index].ItemType}, nil
	}
	methods["RemoveAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.removeAura(spell)
		return nil, nil
	}
	methods["SetCoinage"] = func(ctx context.Context, args []any) ([]any, error) {
		money, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		state.Money = money
		_, err = s.server.CharactersStore.DB.Exec("UPDATE characters SET money = ? WHERE guid = ?", money, state.GUID)
		s.questMoneyChanged(ctx)
		return nil, err
	}
	methods["ModifyMoney"] = func(ctx context.Context, args []any) ([]any, error) {
		delta, err := luaInt64Arg(args, 0)
		if err != nil {
			return nil, err
		}
		money := int64(state.Money) + delta
		if money < 0 {
			money = 0
		}
		if money > math.MaxUint32 {
			money = math.MaxUint32
		}
		state.Money = uint32(money)
		_, err = s.server.CharactersStore.DB.Exec("UPDATE characters SET money = ? WHERE guid = ?", state.Money, state.GUID)
		s.questMoneyChanged(ctx)
		return []any{true}, err
	}
	// Player:GetHonorPoints/GetArenaPoints/SetHonorPoints/SetArenaPoints/
	// ModifyHonorPoints/ModifyArenaPoints (PlayerMethods.h:806-821,
	// 1933-1948, 2160-2178; Player.cpp:7051-7091). The getters read the
	// PLAYER_FIELD_HONOR_CURRENCY / PLAYER_FIELD_ARENA_CURRENCY twins
	// (TotalHonorPoints / ArenaPoints, the same fields the values-update
	// builder broadcasts). The setters cap at CONFIG_MAX_HONOR_POINTS /
	// CONFIG_MAX_ARENA_POINTS and add the known currency when the capped
	// value is nonzero; the modifiers floor at 0 and then run the setter,
	// so the cap applies to them too. Persistence follows the SetCoinage /
	// .modify precedent (immediate characters UPDATE + values-update
	// broadcast); C++ persists via the player save instead, so a crash
	// between the Lua call and the next save loses nothing here that C++
	// would have kept.
	setHonorPoints := func(value uint32) {
		maxPoints := uint32(75000)
		if s.server != nil && s.server.Config.MaxHonorPoints > 0 {
			maxPoints = s.server.Config.MaxHonorPoints
		}
		if value > maxPoints {
			value = maxPoints
		}
		state.TotalHonorPoints = value
		if value > 0 {
			s.addKnownCurrency(state, itemHonorPointsID)
		}
	}
	setArenaPoints := func(value uint32) {
		maxPoints := uint32(10000)
		if s.server != nil && s.server.Config.MaxArenaPoints > 0 {
			maxPoints = s.server.Config.MaxArenaPoints
		}
		if value > maxPoints {
			value = maxPoints
		}
		state.ArenaPoints = value
		if value > 0 {
			s.addKnownCurrency(state, itemArenaPointsID)
		}
	}
	methods["GetHonorPoints"] = luaNoArgs(func() any { return state.TotalHonorPoints })
	methods["GetArenaPoints"] = luaNoArgs(func() any { return state.ArenaPoints })
	methods["SetHonorPoints"] = func(ctx context.Context, args []any) ([]any, error) {
		value, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		setHonorPoints(value)
		s.persistHonorFields(ctx)
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	methods["SetArenaPoints"] = func(ctx context.Context, args []any) ([]any, error) {
		value, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		setArenaPoints(value)
		if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
			_, _ = chars.DB.ExecContext(ctx, "UPDATE characters SET arenaPoints = ? WHERE guid = ?", state.ArenaPoints, state.GUID)
		}
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	methods["ModifyHonorPoints"] = func(ctx context.Context, args []any) ([]any, error) {
		delta, err := luaInt64Arg(args, 0)
		if err != nil || delta > math.MaxInt32 || delta < math.MinInt32 {
			if err == nil {
				err = fmt.Errorf("argument 1 must be int32")
			}
			return nil, err
		}
		value := int64(state.TotalHonorPoints) + delta
		if value < 0 {
			value = 0
		}
		setHonorPoints(uint32(value))
		s.persistHonorFields(ctx)
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	methods["ModifyArenaPoints"] = func(ctx context.Context, args []any) ([]any, error) {
		delta, err := luaInt64Arg(args, 0)
		if err != nil || delta > math.MaxInt32 || delta < math.MinInt32 {
			if err == nil {
				err = fmt.Errorf("argument 1 must be int32")
			}
			return nil, err
		}
		value := int64(state.ArenaPoints) + delta
		if value < 0 {
			value = 0
		}
		setArenaPoints(uint32(value))
		if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
			_, _ = chars.DB.ExecContext(ctx, "UPDATE characters SET arenaPoints = ? WHERE guid = ?", state.ArenaPoints, state.GUID)
		}
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	// Player:GetLifetimeKills/AddLifetimeKills (PlayerMethods.h:1491-1496,
	// 3409-3415): the PLAYER_FIELD_LIFETIME_HONORABLE_KILLS twin is
	// TotalKills (player_state.go values builder). AddLifetimeKills adds
	// with uint32 wrap, like the C++ uint32 addition; neither arm touches
	// today/yesterday kills.
	methods["GetLifetimeKills"] = luaNoArgs(func() any { return state.TotalKills })
	methods["AddLifetimeKills"] = func(ctx context.Context, args []any) ([]any, error) {
		value, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		state.TotalKills += value
		s.persistHonorFields(ctx)
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	// Player:SetLifetimeKills/RemoveLifetimeKills (PlayerMethods.h:2001-2007,
	// 3485-3494): the TotalKills write and the clamped subtraction, both
	// persisted via persistHonorFields like AddLifetimeKills.
	methods["SetLifetimeKills"] = func(ctx context.Context, args []any) ([]any, error) {
		value, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		state.TotalKills = value
		s.persistHonorFields(ctx)
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	methods["RemoveLifetimeKills"] = func(ctx context.Context, args []any) ([]any, error) {
		value, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if value > state.TotalKills {
			value = state.TotalKills
		}
		state.TotalKills -= value
		s.persistHonorFields(ctx)
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	// Player:SetGuildRank (PlayerMethods.h:1745-1754; Player.h:1559):
	// the plain rank-field write, no-op when guildless. C++
	// SetRank writes only PLAYER_GUILDRANK, persisting at the next
	// SaveToDB, so this arm sets the state field and refreshes the
	// client without touching guild_member.
	methods["SetGuildRank"] = func(_ context.Context, args []any) ([]any, error) {
		rank, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if rank > math.MaxUint8 {
			return nil, fmt.Errorf("argument 1 must be uint8")
		}
		if state.GuildID == 0 {
			return []any{}, nil
		}
		state.GuildRank = uint8(rank)
		s.sendPlayerUpdate()
		return []any{}, nil
	}
	// Player:SetPvPDeath (PlayerMethods.h:1836-1842): the s.pvpDeath
	// flag Unit::Kill reads for the corpse-reclaim and durability
	// legs, default true like CHECKVAL<bool>(L, 2, true).
	methods["SetPvPDeath"] = func(_ context.Context, args []any) ([]any, error) {
		on, err := extraBool("SetPvPDeath", args)
		if err != nil {
			return nil, err
		}
		s.pvpDeath = on
		return []any{}, nil
	}
	// Player:SetBindPoint (PlayerMethods.h:2029-2045; Player.cpp:17342):
	// the homebind field write plus UPDATE character_homebind, the
	// C++ CHAR_UPD_PLAYER_HOMEBIND twin. No bind-point packet: C++
	// SetHomebind sends none (SendBindPointUpdate belongs to the
	// innkeeper/spell paths, already bridged there).
	methods["SetBindPoint"] = func(ctx context.Context, args []any) ([]any, error) {
		x, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		y, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		z, err := luaFloat32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		mapID, err := luaUint32Arg(args, 3)
		if err != nil {
			return nil, err
		}
		areaID, err := luaUint32Arg(args, 4)
		if err != nil {
			return nil, err
		}
		state.HomebindMap = mapID
		state.HomebindZone = areaID
		state.HomebindX = x
		state.HomebindY = y
		state.HomebindZ = z
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_homebind SET mapId = ?, zoneId = ?, posX = ?, posY = ?, posZ = ? WHERE guid = ?",
				mapID, areaID, x, y, z, s.playerGUID)
		}
		return []any{}, nil
	}
	// Player:SaveToDB (PlayerMethods.h:2185-2193): the .save command
	// twin, savePlayerState(ctx, 1, false) — online, not a logout save.
	methods["SaveToDB"] = func(ctx context.Context, _ []any) ([]any, error) {
		_ = s.savePlayerState(ctx, 1, false)
		return []any{}, nil
	}
	// Player:SummonPlayer (PlayerMethods.h:2200-2218; Player.cpp:23801):
	// sendSummonRequest over the summoner's live position, with the
	// summoner's zone when it is a live player and the target's zone
	// otherwise. The pending-request and 23445-aura gates live in
	// sendSummonRequest itself.
	methods["SummonPlayer"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("summoner is required")
		}
		summonerGUID := luaObjectGUID(args[0])
		if summonerGUID == 0 {
			return nil, fmt.Errorf("summoner is required")
		}
		zoneID := state.Zone
		if s.server != nil {
			if summonerSess := s.server.findSessionByGUID(summonerGUID); summonerSess != nil && summonerSess.player != nil {
				zoneID = summonerSess.player.Zone
			}
		}
		s.sendSummonRequest(summonerGUID, zoneID)
		return []any{}, nil
	}
	// Player:LogoutPlayer (PlayerMethods.h:2398-2405;
	// WorldSession.cpp:496): the session logout. DELTA: Go's
	// completeLogout always saves, so save=false still persists —
	// there is no m_playerSave gate on the Go logout path.
	methods["LogoutPlayer"] = func(_ context.Context, args []any) ([]any, error) {
		if _, err := extraBool("LogoutPlayer", args); err != nil {
			return nil, err
		}
		s.logout()
		return []any{}, nil
	}
	// Player:KillPlayer (PlayerMethods.h:2630-2634; Player.cpp:4770):
	// the death-state arm. Go's killPlayer is the Unit::Kill tail and
	// no-ops above zero health, so health is zeroed first — the
	// .damage command precedent — then the tail runs with no killer,
	// no PvP-death flag, and no durability loss.
	methods["KillPlayer"] = func(ctx context.Context, _ []any) ([]any, error) {
		if state.Health > 0 {
			state.Health = 0
			s.sendPlayerUpdate()
		}
		s.killPlayer(ctx, nil, false, false)
		return []any{}, nil
	}
	// Player:KickPlayer (PlayerMethods.h:3657-3664;
	// WorldSession.cpp:648): the socket-close kick via kickSession,
	// the luaKickPlayer twin for the self session.
	methods["KickPlayer"] = func(_ context.Context, _ []any) ([]any, error) {
		if s.server != nil {
			s.server.kickSession(s)
		}
		return []any{}, nil
	}
	// Player:ResurrectPlayer (PlayerMethods.h:3729-3737;
	// Player.cpp:4682) plus the Lua arm's SpawnCorpseBones call: the
	// percent is C++ 100-scale, so it is divided by 100 for the Go
	// fraction arm, and toBones=true converts the corpse to bones.
	// The bones arm's triggerSave leg is the SaveToDB call after.
	methods["ResurrectPlayer"] = func(ctx context.Context, args []any) ([]any, error) {
		percent := float32(100)
		if len(args) > 0 {
			value, err := luaFloat32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			percent = value
		}
		sickness := false
		if len(args) > 1 {
			value, ok := args[1].(bool)
			if !ok {
				return nil, fmt.Errorf("argument 2 must be boolean")
			}
			sickness = value
		}
		s.resurrectPlayerWithBones(ctx, percent/100, true, sickness)
		_ = s.savePlayerState(ctx, 1, false)
		return []any{}, nil
	}
	// Player:SpawnBones (PlayerMethods.h:3924-3929;
	// Player.cpp:4872): convertCorpseToBones plus the triggerSave
	// leg, the .revive command shape.
	methods["SpawnBones"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.spawnCorpseBones(ctx)
		_ = s.savePlayerState(ctx, 1, false)
		return []any{}, nil
	}
	// Player:GetReputationRank (PlayerMethods.h:989-997;
	// Player.cpp:6644-6648, ReputationMgr.cpp:119-135): rank of the
	// standing, where an unknown faction reads standing 0 and therefore
	// NEUTRAL (3) in both trees — the shared reputationStanding helper
	// already returns 0 for unlisted factions.
	methods["GetReputationRank"] = func(_ context.Context, args []any) ([]any, error) {
		faction, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{reputationRank(int64(s.reputationStanding(faction)))}, nil
	}
	// Player:SetReputation (PlayerMethods.h:1778-1785): the absolute
	// standing write — see setReputationStanding for the clamp, hook,
	// visibility, and spillover-parked semantics.
	methods["SetReputation"] = func(ctx context.Context, args []any) ([]any, error) {
		faction, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		value, err := luaInt64Arg(args, 1)
		if err != nil || value > math.MaxInt32 || value < math.MinInt32 {
			if err == nil {
				err = fmt.Errorf("argument 2 must be int32")
			}
			return nil, err
		}
		s.setReputationStanding(ctx, faction, int32(value))
		return []any{}, nil
	}
	// Player:GetBattlegroundId/GetBattlegroundTypeId (PlayerMethods.h:
	// 958-977; Player.h:1908-1909) read the m_bgData pair via
	// luaBattlegroundID; outside a battleground both read 0.
	methods["GetBattlegroundId"] = luaNoArgs(func() any {
		id, _ := s.luaBattlegroundID()
		return id
	})
	methods["GetBattlegroundTypeId"] = luaNoArgs(func() any {
		_, typeID := s.luaBattlegroundID()
		return typeID
	})
	// InArena/InBattleground/InBattlegroundQueue are Player::InArena/
	// InBattleground/InBattlegroundQueue (PlayerMethods.h:660-684;
	// Player.h:1906; Player.cpp:23647-23654, 23554-23562): InArena is
	// a live arena roster, InBattleground is the same entered pair
	// GetBattlegroundId reads (bgInstanceID != 0 in C++), and
	// InBattlegroundQueue is any queue slot still waiting or invited
	// (WaitQueue/WaitJoin; entered slots read InProgress, left slots
	// read None, so neither counts as queued).
	methods["InArena"] = luaNoArgs(func() any {
		return s.server != nil && s.server.findArenaStateForPlayer(s.playerGUID) != nil
	})
	methods["InBattleground"] = luaNoArgs(func() any {
		id, _ := s.luaBattlegroundID()
		return id != 0
	})
	methods["InBattlegroundQueue"] = luaNoArgs(func() any {
		for i := range s.bgQueues {
			e := &s.bgQueues[i]
			if e.Active && (e.Status == BGStatusWaitQueue || e.Status == BGStatusWaitJoin) {
				return true
			}
		}
		return false
	})
	// Player:IsInArenaTeam (PlayerMethods.h:430-436; Player.h:1571;
	// ArenaTeam.h:114): the argument is a SLOT index, not a team type —
	// type >= MAX_ARENA_SLOT (3) reads false, and the slot's team id
	// (ArenaTeamInfo base + 0, the values-builder twin kept current by
	// loadArenaTeamInfo and the live set/clear slot helpers) decides.
	methods["IsInArenaTeam"] = func(_ context.Context, args []any) ([]any, error) {
		slot, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if slot >= 3 {
			return []any{false}, nil
		}
		return []any{state.ArenaTeamInfo[slot*7] != 0}, nil
	}
	methods["SetGender"] = func(_ context.Context, args []any) ([]any, error) {
		gender, err := luaUint32Arg(args, 0)
		if err != nil || gender > 1 {
			if err == nil {
				err = fmt.Errorf("gender must be 0 or 1")
			}
			return nil, err
		}
		state.Gender = uint8(gender)
		_, err = s.server.CharactersStore.DB.Exec("UPDATE characters SET gender = ? WHERE guid = ?", state.Gender, state.GUID)
		return nil, err
	}
	methods["SetHealth"] = func(_ context.Context, args []any) ([]any, error) {
		health, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if health > state.MaxHealth {
			health = state.MaxHealth
		}
		state.Health = health
		_, err = s.server.CharactersStore.DB.Exec("UPDATE characters SET health = ? WHERE guid = ?", health, state.GUID)
		return nil, err
	}
	// SetStandState is Unit::SetStandState (Unit.cpp:10557-10569):
	// SetByteValue(UNIT_FIELD_BYTES_1, stand-state byte) + the
	// SMSG_STANDSTATE_UPDATE send to the player + values-update broadcast
	// (emote.go handleStandStateChange twin); standing (0) breaks
	// NOT_SEATED-interrupt auras. Validated 0-9 like the creature twin
	// (lua_creature_events.go:1621). Backs the frostblade-shrine sit arm
	// (go_scripts.cpp:739-753).
	methods["SetStandState"] = func(_ context.Context, args []any) ([]any, error) {
		st, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if st > 9 {
			return nil, fmt.Errorf("SetStandState requires a UNIT_STAND_STATE 0-9, got %d", st)
		}
		state.StandState = uint8(st)
		if st == 0 {
			s.removeAurasWithInterruptFlags(auraInterruptFlagNotSeated)
		}
		merged := uint32(st) | uint32(state.StandFlags)<<16
		buf := protocol.NewBuffer(1)
		buf.WriteU8(uint8(st))
		_ = s.write(uint16(protocol.OpcodeSMSG_STANDSTATE_UPDATE), buf.Bytes(), true)
		s.server.broadcastPlayerValuesUpdateFromSession(s, map[int]uint32{unitFieldBytes1: merged})
		return nil, nil
	}
	// GetStandState is Unit::GetStandState (Unit.h:921): the
	// UNIT_FIELD_BYTES_1 stand-state byte SetStandState writes.
	methods["GetStandState"] = luaNoArgs(func() any { return state.StandState })
	// IsStandState is Unit::IsStandState (Unit.cpp:10551-10555),
	// shared predicate unitIsStandState (lua_creature_events.go).
	methods["IsStandState"] = luaNoArgs(func() any { return unitIsStandState(state.StandState) })
	// IsRooted is Unit::IsRooted (UnitMethods.h, Trinity arm):
	// UNIT_STATE_ROOT or MOVEMENTFLAG_ROOT. Go's twin is the
	// session rooted flag (setRooted + login root-aura scan,
	// characters.go); a bare movement-flag root has no Go model.
	methods["IsRooted"] = luaNoArgs(func() any { return s.rooted })
	methods["LearnSpell"] = func(ctx context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		query := "INSERT OR IGNORE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)"
		if s.server.CharactersStore.Backend != "sqlite" {
			query = "INSERT IGNORE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)"
		}
		_, err = s.server.CharactersStore.DB.Exec(query, state.GUID, spell)
		s.learnOwnerPetAuraSources(ctx, spell)
		return nil, err
	}
	methods["RemoveSpell"] = func(ctx context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		_, err = s.server.CharactersStore.DB.Exec("DELETE FROM character_spell WHERE guid = ? AND spell = ?", state.GUID, spell)
		if s.hasAura(spell) {
			s.removeAura(spell)
		}
		s.removeOwnerPetAurasForSpell(ctx, spell)
		return nil, err
	}
	methods["HasQuest"] = func(_ context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		var count int64
		err = s.server.CharactersStore.DB.QueryRow("SELECT COUNT(1) FROM character_queststatus WHERE guid = ? AND quest = ?", state.GUID, quest).Scan(&count)
		if isMissingTableError(err) {
			return []any{false}, nil
		}
		return []any{err == nil && count > 0}, err
	}
	// GetQuestStatus is Player::GetQuestStatus (Player.cpp:16131-16143)
	// via the session characterQuestStatus twin (quests.go): NONE 0,
	// COMPLETE 1, INCOMPLETE 3, FAILED 5, REWARDED 6 (QuestDef.h:101-109).
	methods["GetQuestStatus"] = func(lctx context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return []any{int64(0)}, nil
		}
		status, _ := s.characterQuestStatus(lctx, quest)
		return []any{status}, nil
	}
	// GetQuestRewardStatus is Player::GetQuestRewardStatus
	// (Player.cpp:16115-16128): for a non-seasonal, non-repeatable quest
	// it is IsQuestRewarded (Player.cpp:22854), the rewarded-set membership
	// the characterQuestStatus twin (quests.go) already reads from
	// character_queststatus_rewarded (active = 1); repeatable quests return
	// false in C++, and the rewarded-row predicate already mirrors the
	// repeatable/daily/DF exclusions. The seasonal leg
	// (!SatisfyQuestSeasonal) is unmodeled: a seasonal quest reports its
	// rewarded-row membership here. Read directly from the rewarded set,
	// not via characterQuestStatus, because C++ checks IsQuestRewarded
	// independently of the active status map.
	methods["GetQuestRewardStatus"] = func(_ context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return []any{false}, nil
		}
		var count int64
		err = s.server.CharactersStore.DB.QueryRow("SELECT COUNT(1) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ? AND active = 1", state.GUID, quest).Scan(&count)
		if isMissingTableError(err) {
			return []any{false}, nil
		}
		return []any{err == nil && count > 0}, err
	}
	methods["CompleteQuest"] = func(_ context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		_, err = s.server.CharactersStore.DB.Exec("UPDATE character_queststatus SET status = 1 WHERE guid = ? AND quest = ?", state.GUID, quest)
		if isMissingTableError(err) {
			return nil, nil
		}
		return nil, err
	}
	// CanCompleteQuest is Player::CanCompleteQuest (Player.cpp:14970-15040)
	// via the elunaCanCompleteQuest twin below: the rewarded gate, the
	// auto-complete arm, then the INCOMPLETE objective checks against the
	// stored status row (not live inventory — C++ reads q_status.ItemCount,
	// while the accept-path canCompleteQuest twin reads live counts).
	methods["CanCompleteQuest"] = func(lctx context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return []any{false}, nil
		}
		return []any{s.elunaCanCompleteQuest(lctx, quest)}, nil
	}
	// GetQuestLevel is Player::GetQuestLevel (Player.h:1207): the quest
	// template level when positive, else the player level. The Eluna
	// argument is a Quest object, so its ID is resolved to a signed
	// template level first; the object's Level field is only the
	// no-DB fallback, re-signed so a wrapped negative level still falls
	// back to the player level instead of reading as a huge uint32.
	methods["GetQuestLevel"] = func(lctx context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("GetQuestLevel requires a quest")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("GetQuestLevel requires a quest")
		}
		questID, ok := objectUint32Field(obj, "ID")
		if !ok {
			return nil, fmt.Errorf("GetQuestLevel requires a quest")
		}
		var playerLevel int64
		if state != nil {
			playerLevel = int64(state.Level)
		}
		if s != nil && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			var level int64
			if err := s.server.WorldStore.DB.QueryRowContext(lctx, "SELECT COALESCE(QuestLevel, 0) FROM quest_template WHERE ID = ?", questID).Scan(&level); err == nil {
				if level > 0 {
					return []any{level}, nil
				}
				return []any{playerLevel}, nil
			}
		}
		if levelField, ok := objectUint32Field(obj, "Level"); ok {
			if level := int64(int32(levelField)); level > 0 {
				return []any{level}, nil
			}
		}
		return []any{playerLevel}, nil
	}
	// GetReqKillOrCastCurrentCount is Player::GetReqKillOrCastCurrentCount
	// (Player.cpp:16397-16411): the stored CreatureOrGOCount for the first
	// objective slot whose RequiredNpcOrGo equals the entry, else 0. No
	// status filter: C++ returns the count for any recorded status.
	methods["GetReqKillOrCastCurrentCount"] = func(lctx context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		entry, err := luaInt32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return []any{int64(0)}, nil
		}
		data, ok := s.luaQuestObjectives(lctx, quest)
		if !ok {
			return []any{int64(0)}, nil
		}
		row := s.luaQuestStatusRow(lctx, quest)
		if !row.found {
			return []any{int64(0)}, nil
		}
		for index := range data.RequiredNpcOrGo {
			if data.RequiredNpcOrGo[index] == entry {
				return []any{int64(row.counters[index])}, nil
			}
		}
		return []any{int64(0)}, nil
	}
	// HasQuestForGO is Player::HasQuestForGO (Player.cpp:23725-23757):
	// an INCOMPLETE quest in the log whose RequiredNpcOrGo slot is the
	// negated GO entry with an unmet count. The raid-group hide arm
	// (raid group + quest not allowed in raid + not in battleground) is
	// unmodeled: the Go group model exposes no raid/difficulty gate on
	// this path, so in non-raid play the arm is a no-op anyway.
	methods["HasQuestForGO"] = func(lctx context.Context, args []any) ([]any, error) {
		entry, err := luaInt32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return []any{false}, nil
		}
		for _, questID := range s.luaQuestLogIDs(lctx) {
			row := s.luaQuestStatusRow(lctx, questID)
			if !row.found || row.status != questStatusIncomplete {
				continue
			}
			data, ok := s.luaQuestObjectives(lctx, questID)
			if !ok {
				continue
			}
			for index := range data.RequiredNpcOrGo {
				if data.RequiredNpcOrGo[index] >= 0 {
					continue
				}
				if -entry == data.RequiredNpcOrGo[index] && int64(row.counters[index]) < int64(data.RequiredNpcOrGoCount[index]) {
					return []any{true}, nil
				}
			}
		}
		return []any{false}, nil
	}
	// HasQuestForItem is Player::HasQuestForItem (Player.cpp:16945-17000)
	// with the Eluna defaults excludeQuestId 0 and turnIn false: an
	// INCOMPLETE quest in the log whose required-item count is unmet, or
	// whose source item (ItemDrop) is still droppable — unique items by
	// MaxCount, others by ItemDropQuantity or the stack size — against
	// the inventory-plus-bank count. The raid-group hide arm is unmodeled
	// for the same reason as HasQuestForGO.
	methods["HasQuestForItem"] = func(lctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return []any{false}, nil
		}
		for _, questID := range s.luaQuestLogIDs(lctx) {
			row := s.luaQuestStatusRow(lctx, questID)
			if !row.found || row.status != questStatusIncomplete {
				continue
			}
			data, ok := s.luaQuestObjectives(lctx, questID)
			if !ok {
				continue
			}
			for index := range data.RequiredItemID {
				if data.RequiredItemID[index] == entry && int64(row.itemCounts[index]) < int64(data.RequiredItemCount[index]) {
					return []any{true}, nil
				}
			}
			for index := range data.ItemDrop {
				if data.ItemDrop[index] != entry {
					continue
				}
				if s.luaQuestSourceItemDroppable(lctx, questID, index, entry) {
					return []any{true}, nil
				}
			}
		}
		return []any{false}, nil
	}
	// SetQuestStatus is Player::SetQuestStatus (Player.cpp:16166-16180)
	// with update true: a status at or above MAX_QUEST_STATUS (7) is a
	// no-op, a missing template writes no row, otherwise the status is
	// written preserving the objective counts, then SendQuestUpdate and
	// the OnQuestStatusChange hook run. Unlike IncompleteQuest it does
	// not touch the log slot's complete flag — C++ SetQuestStatus
	// doesn't either.
	methods["SetQuestStatus"] = func(lctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		status, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if status >= 7 || s == nil {
			return nil, nil
		}
		if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && !characterQuestTemplateExists(lctx, s.server.WorldStore.DB, entry) {
			s.sendQuestUpdate(lctx, entry)
			s.triggerPlayerEvent(lctx, scripting.PlayerEventQuestStatusChange, s.luaPlayer(), entry)
			return nil, nil
		}
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			cdb := s.server.CharactersStore.DB
			res, err := cdb.ExecContext(lctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", status, s.playerGUID, entry)
			if err == nil {
				if affected, affErr := res.RowsAffected(); affErr == nil && affected == 0 {
					_, _ = cdb.ExecContext(lctx, "INSERT INTO character_queststatus (guid, quest, status) VALUES (?, ?, ?)", s.playerGUID, entry, status)
				}
			}
		}
		s.sendQuestUpdate(lctx, entry)
		s.triggerPlayerEvent(lctx, scripting.PlayerEventQuestStatusChange, s.luaPlayer(), entry)
		return nil, nil
	}
	// IncompleteQuest is Player::IncompleteQuest (Player.cpp:15320-15329)
	// via the session incompleteQuest twin (quest_details.go): the
	// status drops back to INCOMPLETE and the log slot's complete flag
	// clears. A zero quest id is a no-op in C++.
	methods["IncompleteQuest"] = func(lctx context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if quest != 0 && s != nil {
			s.incompleteQuest(lctx, quest)
		}
		return nil, nil
	}
	// AddQuest is LuaPlayer::AddQuest (PlayerMethods.h:2953-3011): a
	// missing template or an item-start quest (any item_template row
	// with StartQuest equal to the entry) is a no-op; otherwise
	// CanAddQuest (log space + source-item storability, the session
	// canAddQuest twin) gates AddQuestAndCheckCompletion, which the
	// session addQuestToPlayer twin already performs end to end with a
	// nil giver (no questgiver hooks, matching the Eluna NULL giver).
	// The completion check after it runs the Eluna stored-count variant
	// so a quest the accept-path twin left incomplete on live-inventory
	// grounds still completes when the stored counts satisfy C++.
	methods["AddQuest"] = func(lctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, nil
		}
		if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			if !characterQuestTemplateExists(lctx, s.server.WorldStore.DB, entry) {
				return nil, nil
			}
			var itemEntry int64
			if err := s.server.WorldStore.DB.QueryRowContext(lctx, "SELECT entry FROM item_template WHERE StartQuest = ? LIMIT 1", entry).Scan(&itemEntry); err == nil {
				return nil, nil
			}
		}
		if !s.canAddQuest(lctx, entry, true) {
			return nil, nil
		}
		s.addQuestToPlayer(lctx, entry, 0)
		if s.elunaCanCompleteQuest(lctx, entry) {
			s.completeQuest(lctx, entry)
		}
		return nil, nil
	}
	// RemoveQuest is LuaPlayer::RemoveQuest (PlayerMethods.h:3013-3061):
	// a missing template is a no-op; otherwise every log slot holding
	// the entry is cleared with TakeQuestSourceItem, then
	// RemoveActiveQuest (status row delete, no quest update) and
	// RemoveRewardedQuest (rewarded row delete, quest update + hook)
	// run. The PvP-flag recompute arm has no Go twin on the remove path
	// (the accept path forces it via applyZoneState), and the seasonal
	// runtime-set erase has no DB counterpart — the seasonal cooldown
	// rows in character_queststatus_seasonal are cooldown records, not
	// the active set, so they are preserved.
	methods["RemoveQuest"] = func(lctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, nil
		}
		if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && !characterQuestTemplateExists(lctx, s.server.WorldStore.DB, entry) {
			return nil, nil
		}
		if s.player != nil {
			for slot := 0; slot < playerQuestLogSlots; slot++ {
				if s.player.QuestLog[slot].QuestID == entry {
					s.player.QuestLog[slot] = questLogEntry{}
					s.sendPlayerQuestLogUpdate(slot)
					s.takeQuestSourceItem(lctx, entry, false)
				}
			}
		}
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(lctx, "DELETE FROM character_queststatus WHERE guid = ? AND quest = ?", s.playerGUID, entry)
			_, _ = s.server.CharactersStore.DB.ExecContext(lctx, "DELETE FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", s.playerGUID, entry)
		}
		s.sendQuestUpdate(lctx, entry)
		s.triggerPlayerEvent(lctx, scripting.PlayerEventQuestStatusChange, s.luaPlayer(), entry)
		return nil, nil
	}
	// FailQuest mirrors Player::FailQuest (Player.cpp:15529-15577) via the
	// session failQuest twin (quest_details.go): only an INCOMPLETE quest
	// (or a COMPLETE timed quest with COMPLETED_AT_START) transitions;
	// anything else is a no-op, so a wrong raid-size quest id is harmless.
	methods["FailQuest"] = func(lctx context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s != nil {
			s.failQuest(lctx, quest)
		}
		return nil, nil
	}
	// AreaExploredOrEventHappens mirrors Player::AreaExploredOrEventHappens
	// (Player.cpp:16503-16527) via the session areaExploredOrEventHappens
	// twin (quests.go): the explored flip plus the CanCompleteQuest-gated
	// completion, so a quest whose objectives are not yet done is NOT
	// completed by this call.
	methods["AreaExploredOrEventHappens"] = func(lctx context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s != nil {
			s.areaExploredOrEventHappens(lctx, quest)
		}
		return nil, nil
	}
	// KilledMonsterCredit runs Player::KilledMonsterCredit
	// (Player.cpp:16636) via the session grantKillCreditMonster twin
	// (kill.go): timed achievement, KILL_CREATURE criteria, then the quest
	// loop. targetGUID 0 matches the C++ pRewardSource arm (real_entry ==
	// entry when the creature GUID is empty).
	methods["KilledMonsterCredit"] = func(lctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s != nil {
			s.grantKillCreditMonster(lctx, entry, 0)
		}
		return nil, nil
	}
	// TalkedToCreature is Player::TalkedToCreature
	// (PlayerMethods.h:2742-2750, Player.cpp:16802) via the session
	// talkedToCreature twin (quests.go): the creature argument is
	// required like Eluna's CHECKOBJ, and its GUID rides the quest
	// update packet; a non-creature object still credits by entry, since
	// C++ only reads the creature's GUID, never its entry.
	methods["TalkedToCreature"] = func(lctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if len(args) < 2 {
			return nil, fmt.Errorf("TalkedToCreature requires a creature")
		}
		obj, ok := args[1].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("TalkedToCreature requires a creature")
		}
		guid, _ := objectUint64Field(obj, "GUID")
		if s != nil {
			s.talkedToCreature(lctx, entry, guid)
		}
		return nil, nil
	}
	// GroupEventHappens is Player::GroupEventHappens
	// (PlayerMethods.h:2770-2778, Player.cpp:16527) via the session
	// groupEventHappens twin (quests.go): the world-object argument is
	// required like Eluna's CHECKOBJ and anchors the group reward
	// distance test.
	methods["GroupEventHappens"] = func(lctx context.Context, args []any) ([]any, error) {
		quest, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if len(args) < 2 {
			return nil, fmt.Errorf("GroupEventHappens requires a worldobject")
		}
		obj, ok := args[1].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("GroupEventHappens requires a worldobject")
		}
		if s != nil {
			s.groupEventHappens(lctx, quest, obj)
		}
		return nil, nil
	}
	// RewardQuest is LuaPlayer::RewardQuest (PlayerMethods.h:2247-2261):
	// a missing template or a quest whose status is not COMPLETE is a
	// silent no-op; otherwise the full Player::RewardQuest grant path
	// runs with no choice item and announce=true — the .quest reward
	// command twin (commands_quest.go), giver 0 so no chain quest or
	// giver-status packet rides along, matching the GM path. The
	// command's quest-disabled gate is not applied: the Eluna arm has
	// none. The commit's claimed-window gate can still refuse a
	// daily/weekly already claimed in its window, silently like the
	// template/status gates.
	methods["RewardQuest"] = func(lctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return nil, nil
		}
		st, err := s.characterQuestStatus(lctx, entry)
		if err != nil || st != questStatusComplete {
			return nil, nil
		}
		view, err := s.loadQuestRewardView(lctx, entry)
		if err != nil {
			return nil, nil
		}
		grants, destroyedGUIDs, err := s.commitQuestReward(lctx, view, 0)
		if err != nil {
			return nil, nil
		}
		s.applyQuestRewardGrants(lctx, view, entry, 0, 0, grants, destroyedGUIDs, true)
		return nil, nil
	}
	methods["SendBroadcastMessage"] = s.luaMessageMethod()
	methods["SendAreaTriggerMessage"] = s.luaMessageMethod()
	methods["SendNotification"] = s.luaMessageMethod()
	methods["Teleport"] = func(lctx context.Context, args []any) ([]any, error) {
		if len(args) < 5 {
			return nil, fmt.Errorf("Teleport requires map and position")
		}
		mapID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		position := make([]float32, 4)
		for index := range position {
			value, valueErr := luaFloat32Arg(args, index+1)
			if valueErr != nil {
				return nil, valueErr
			}
			position[index] = value
		}
		state.Map, state.X, state.Y, state.Z, state.Orientation = mapID, position[0], position[1], position[2], position[3]
		packet := protocol.NewBuffer(20)
		packet.WriteU32(state.Map)
		packet.WriteF32(state.X)
		packet.WriteF32(state.Y)
		packet.WriteF32(state.Z)
		packet.WriteF32(state.Orientation)
		if err := s.write(uint16(protocol.OpcodeSMSG_NEW_WORLD), packet.Bytes(), true); err != nil {
			return nil, err
		}
		// Player::TeleportTo (Player.cpp:1878): SendSavedInstances follows
		// SMSG_NEW_WORLD on a far teleport.
		s.sendSavedInstances(lctx)
		return nil, nil
	}
	// TeleportNear is the same-map arm of Teleport: it runs the
	// near-teleport path (movement.go nearTeleportMove — the Go analog of
	// the same-map Player::TeleportTo branch used by ScriptedAI's
	// DoTeleportPlayer) instead of the far-transfer SMSG_NEW_WORLD above.
	// Same-map script teleports (e.g. Jin'do's EVENT_TELEPORT) must not
	// trigger a loading screen.
	methods["TeleportNear"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 4 {
			return nil, fmt.Errorf("TeleportNear requires position")
		}
		position := make([]float32, 4)
		for index := range position {
			value, valueErr := luaFloat32Arg(args, index)
			if valueErr != nil {
				return nil, valueErr
			}
			position[index] = value
		}
		// Player::TeleportTo (Player.cpp:1703-1704) leaves the vehicle
		// before any teleport, the same arm teleportToOptions runs;
		// without it a script-teleported passenger kept a stale
		// VehicleGUID pointing at a vehicle on the origin spot while
		// the ack moved only the player.
		s.exitVehicle(context.Background())
		s.nearTeleportMove(position[0], position[1], position[2], position[3], false)
		return nil, nil
	}
	// NearTeleport is Unit::NearTeleport (UnitMethods.h:2690) for
	// players: NearTeleportTo's player arm (Unit.cpp:13332) is the
	// same-map TeleportTo with the NOT_LEAVE flags, which is exactly
	// what TeleportNear runs via nearTeleportMove — so it aliases it,
	// keeping the all-four-required shape that matches NearTeleport's
	// four CHECKVALs.
	methods["NearTeleport"] = methods["TeleportNear"]
	// GetSpeed is UnitMethods::GetSpeed (UnitMethods.h:1392-1404) for
	// players: Unit::GetSpeed (Unit.cpp:8811-8814) = rate *
	// playerBaseMoveSpeed[type]. Go's movementSpeeds() (movement_speed.go)
	// is that product already folded: the aura-derived absolute speeds,
	// shadowed by any live GM/script rate override (gmSpeedOverride).
	// All 9 UnitMoveType indices resolve here, unlike the creature
	// surface's WALK/RUN/FLIGHT-only model — players carry no
	// template-speed gap. A type >= MAX_MOVE_TYPE (9) errors, mirroring
	// C++'s luaL_argerror "valid UnitMoveType expected".
	methods["GetSpeed"] = func(_ context.Context, args []any) ([]any, error) {
		mt, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if mt >= 9 {
			return nil, fmt.Errorf("GetSpeed requires a valid UnitMoveType, got %d", mt)
		}
		return []any{s.movementSpeeds()[mt]}, nil
	}
	// SetSpeed is UnitMethods::SetSpeed (UnitMethods.h:1532-1546) for
	// players: the Trinity arm calls SetSpeedRate(type, rate) directly
	// (the forced flag is voided in C++), so this rides setGMSpeedRate
	// (movement_speed.go), the Unit::SetSpeedRate twin: the no-op-on-
	// unchanged gate, the forced-speed-change ACK registration, and the
	// SMSG_FORCE_*_SPEED_CHANGE to self plus MSG_MOVE_SET_*_SPEED to
	// nearby. A rate <= 0 follows the creature SetSpeedRate precedent
	// instead of C++'s clamp-to-0 store: it clears the script override
	// and re-sends the aura-derived speed, so GetSpeed and the client
	// agree (C++ would hold speed 0 until the next UpdateSpeed).
	// DELTAS: PropagateSpeedChange is MotionMaster-only (no Go bridge);
	// the pet propagation (Unit.cpp:8855, out-of-combat only) needs no
	// bridge since Go pets have no speed model; an aura-driven recompute
	// for the type clears the override, exactly like C++ UpdateSpeed
	// overwriting m_speed_rate.
	methods["SetSpeed"] = func(_ context.Context, args []any) ([]any, error) {
		mt, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if mt >= 9 {
			return nil, fmt.Errorf("SetSpeed requires a valid UnitMoveType, got %d", mt)
		}
		rate, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if rate <= 0 {
			if s.gmSpeedRates[int(mt)] != 0 {
				s.clearGMSpeedRate(int(mt))
				opcodes := speedForceOpcodes[mt]
				s.sendRuntimeMovementSpeed(opcodes[0], opcodes[1], s.movementSpeeds()[mt], mt == moveTypeRun)
			}
			return nil, nil
		}
		s.setGMSpeedRate(int(mt), rate)
		return nil, nil
	}
	// SetSpeedRate is the same rate-form bridge the creature surface
	// exposes; Eluna binds only SetSpeed, but scripts calling the C++
	// name directly get the identical arm.
	methods["SetSpeedRate"] = methods["SetSpeed"]
	// applyPlayerFacing is the shared SetFacing/SetFacingToObject tail:
	// C++ Unit::SetFacingTo (Unit.cpp:13669) launches an in-place
	// MoveTo+SetFacing spline, but Go has no player spline, so the twin
	// writes state.Orientation and replays the current position with the
	// new facing as a MSG_MOVE_HEARTBEAT to self and the visible set —
	// the sendTransportMovement precedent (transports.go) for
	// server-driven player movement. DELTA: the C++ IsStopped gate is
	// ungated, same delta as the creature twin; the spline turn is a
	// snap, not an interpolated turn.
	applyPlayerFacing := func(ori float32) {
		s.player.Orientation = ori
		info := s.movementInfoForCreate(*s.player)
		packet := protocol.NewBuffer(96)
		writeMovementInfo(packet, info)
		_ = s.write(uint16(protocol.OpcodeMSG_MOVE_HEARTBEAT), packet.Bytes(), true)
		if s.server != nil {
			s.server.broadcastMovement(uint16(protocol.OpcodeMSG_MOVE_HEARTBEAT), packet.Bytes(), info, s)
		}
	}
	// SetFacing is UnitMethods::SetFacing (UnitMethods.h:1766-1771) for
	// players: SetFacingTo(o) verbatim, via applyPlayerFacing above.
	methods["SetFacing"] = func(_ context.Context, args []any) ([]any, error) {
		ori, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		applyPlayerFacing(ori)
		return nil, nil
	}
	// SetFacingTo is the C++ Unit::SetFacingTo name, aliased to SetFacing
	// like the creature surface does.
	methods["SetFacingTo"] = methods["SetFacing"]
	// SetFacingToObject is UnitMethods::SetFacingToObject
	// (UnitMethods.h:1778-1783) for players: Unit::SetFacingToObject
	// computes the absolute angle to the target (Unit.cpp:13677, the
	// GetAbsoluteAngle twin: atan2 normalized to [0, 2pi)) and turns in
	// place, so this resolves the target's X/Y through the object fields
	// (creature/player/GO objects all carry them) and rides the same
	// heartbeat tail. An argument without X/Y fields errors (the Eluna
	// CHECKOBJ leg).
	methods["SetFacingToObject"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("SetFacingToObject requires a WorldObject target")
		}
		target, ok := args[0].(*scripting.Object)
		if !ok || target == nil {
			return nil, fmt.Errorf("SetFacingToObject requires a WorldObject target")
		}
		tx, ok := objectFloat32Field(target, "X")
		if !ok {
			return nil, fmt.Errorf("SetFacingToObject requires a WorldObject target with a position")
		}
		ty, ok := objectFloat32Field(target, "Y")
		if !ok {
			return nil, fmt.Errorf("SetFacingToObject requires a WorldObject target with a position")
		}
		angle := float32(math.Atan2(float64(ty-s.player.Y), float64(tx-s.player.X)))
		if angle < 0 {
			angle += 2 * math.Pi
		}
		applyPlayerFacing(angle)
		return nil, nil
	}
	methods["Mute"] = func(_ context.Context, args []any) ([]any, error) {
		seconds, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		muteTime := time.Now().Unix() + int64(seconds)
		_, err = s.server.AuthStore.DB.Exec("UPDATE account SET mutetime = ? WHERE id = ?", muteTime, s.accountID)
		s.muteTime = muteTime
		return nil, err
	}
	methods["GetSelection"] = func(ctx context.Context, _ []any) ([]any, error) {
		if s.selection == 0 {
			return []any{nil}, nil
		}
		if player := s.server.findPlayer(s.selection); player != nil {
			return []any{player}, nil
		}
		if creature := s.luaCreature(ctx, s.selection); creature != nil {
			return []any{creature}, nil
		}
		if object := s.luaGameObject(ctx, s.selection); object != nil {
			return []any{object}, nil
		}
		return []any{nil}, nil
	}
	// GetScale is Object::GetScale (ObjectMethods.h:134; Object.h:87):
	// GetFloatValue(OBJECT_FIELD_SCALE_X), which the player
	// indexed-value backend reads as s.scale while set, else the
	// aura-folded native scale (lua_values.go). It mirrors that read
	// exactly, so GetScale and GetFloatValue(OBJECT_FIELD_SCALE_X)
	// never disagree. SetScale below still broadcasts nothing; the
	// indexed-value SetFloatValue path is the broadcasting writer.
	methods["GetScale"] = luaNoArgs(func() any {
		if s.scale != 0 {
			return float64(s.scale)
		}
		return float64(s.auraObjectScale())
	})
	methods["SetScale"] = func(_ context.Context, args []any) ([]any, error) {
		value, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.scale = value
		return nil, nil
	}
	methods["EmoteState"] = func(_ context.Context, args []any) ([]any, error) {
		value, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.emoteState = value
		return nil, nil
	}
	methods["SetPlayerLock"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("SetPlayerLock requires a boolean")
		}
		value, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetPlayerLock requires a boolean")
		}
		s.playerLocked = value
		return nil, nil
	}
	methods["GossipComplete"] = s.luaGossipComplete
	methods["GossipClearMenu"] = s.luaGossipClearMenu
	methods["GossipMenuAddItem"] = s.luaGossipMenuAddItem
	methods["GossipSendMenu"] = s.luaGossipSendMenu
	// GetGossipTextId is Player::GetGossipTextId (PlayerMethods.h:1405-1410;
	// Player.cpp:14703-14727): the gossip_menu TextID for the source's
	// default menu, DEFAULT_GOSSIP_MESSAGE (0x00FFFFFF) when the source
	// has no menu or no row's conditions pass. The menu scan is the
	// shared loadGossipMenuTitleID arm (gossip.go); the default menu
	// comes from the creature object's GossipMenuID field (or a
	// creature_template lookup for event-surface objects, which carry
	// no such field) and, for gameobjects, from the questgiver/goober
	// template arm (Player.cpp:14729-14739). A non-creature/non-GO
	// source reads the default, matching the C++ default arm (menu 0);
	// a missing or non-object arg errors like Eluna's CHECKOBJ.
	methods["GetGossipTextId"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("worldobject is required")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("worldobject is required")
		}
		entry, ok := objectUint32Field(obj, "Entry")
		if !ok {
			return nil, fmt.Errorf("worldobject is required")
		}
		guid, _ := objectUint64Field(obj, "GUID")
		var menuID uint32
		switch obj.Type {
		case "Creature":
			if id, ok := objectUint32Field(obj, "GossipMenuID"); ok {
				menuID = id
			} else if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
				var gossipMenuID int64
				if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT gossip_menu_id FROM creature_template WHERE entry = ?", entry).Scan(&gossipMenuID); err == nil {
					menuID = uint32(gossipMenuID)
				}
			}
		case "GameObject":
			var goType int64
			if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
				_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT type FROM gameobject_template WHERE entry = ?", entry).Scan(&goType)
			}
			menuID = s.gameObjectDefaultGossipMenuID(ctx, entry, uint8(goType))
		default:
			return []any{uint32(0x00FFFFFF)}, nil
		}
		textID, err := s.loadGossipMenuTitleID(ctx, menuID, entry, guid)
		if err != nil {
			return nil, err
		}
		return []any{textID}, nil
	}
	// NpcBot hire-state queries for the botgiver Lua port
	// (lua_scripts/world/botgiver.lua, port of script_bot_giver in
	// src/server/game/AI/NpcBots/botgiver.cpp). The gossip-hire flow is the
	// only C++ caller of BotMgr::AddBot(bot, takeMoney=true); the paid-hire
	// entry point it drives is NPCBotManager.Recruit.
	npcBots := func() *NPCBotManager {
		if s == nil || s.server == nil || s.server.Features == nil {
			return nil
		}
		return s.server.Features.NPCBots
	}
	methods["IsNpcBotEnabled"] = luaNoArgs(func() any {
		mgr := npcBots()
		return mgr != nil && mgr.Config().Enable
	})
	methods["GetNpcBotsCount"] = luaNoArgs(func() any {
		mgr := npcBots()
		if mgr == nil {
			return uint32(0)
		}
		return uint32(mgr.CountByOwner(uint32(s.playerGUID)))
	})
	methods["GetMaxNpcBots"] = luaNoArgs(func() any {
		mgr := npcBots()
		if mgr == nil {
			return uint32(0)
		}
		return mgr.MaxNPCBots()
	})
	methods["IsNpcBotClassEnabled"] = func(_ context.Context, args []any) ([]any, error) {
		class, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mgr := npcBots()
		if mgr == nil {
			return []any{false}, nil
		}
		return []any{mgr.IsClassEnabled(uint8(class))}, nil
	}
	methods["GetMaxNpcBotsPerClass"] = luaNoArgs(func() any {
		mgr := npcBots()
		if mgr == nil {
			return uint32(0)
		}
		return mgr.MaxNPCBotsPerClass()
	})
	methods["GetNpcBotClassCount"] = func(_ context.Context, args []any) ([]any, error) {
		class, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mgr := npcBots()
		if mgr == nil {
			return []any{uint32(0)}, nil
		}
		return []any{mgr.ClassBotCount(uint32(s.playerGUID), uint8(class))}, nil
	}
	// GetFreeNpcBotCount(class) / GetFreeNpcBot(class, index) expose the
	// hireable bots of one class == the botgiver HIRE_CLASS menu loop over
	// _existingBots; names come from creature_template. Index-based (rather
	// than a table) because the Lua value pusher only handles scalars.
	methods["GetFreeNpcBotCount"] = func(_ context.Context, args []any) ([]any, error) {
		class, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mgr := npcBots()
		if mgr == nil {
			return []any{uint32(0)}, nil
		}
		return []any{uint32(len(mgr.FreeBotEntries(uint8(class))))}, nil
	}
	methods["GetFreeNpcBot"] = func(ctx context.Context, args []any) ([]any, error) {
		class, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		index, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		mgr := npcBots()
		if mgr == nil {
			return []any{uint32(0), ""}, nil
		}
		entries := mgr.FreeBotEntries(uint8(class))
		if int(index) >= len(entries) {
			return []any{uint32(0), ""}, nil
		}
		entry := entries[index]
		return []any{entry, s.npcbotTemplateName(ctx, entry)}, nil
	}
	// GetNpcBotName(entry) resolves a bot entry to its creature_template
	// name == Creature::GetName() for the HIRE_ENTRY busy whisper.
	methods["GetNpcBotName"] = func(ctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{s.npcbotTemplateName(ctx, entry)}, nil
	}
	// GetNpcBotClass(entry) is bot_ai::_botclass for the per-bot hire
	// menu (lua_scripts/world/bothire.lua, port of the bot_ai.cpp
	// free-bot hire arms at :5279-5336 + :7202-7270): 0 when the entry
	// is not a known bot.
	methods["GetNpcBotClass"] = func(_ context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mgr := npcBots()
		if mgr == nil {
			return []any{uint32(0)}, nil
		}
		extras, ok := mgr.Extras(entry)
		if !ok {
			return []any{uint32(0)}, nil
		}
		return []any{uint32(extras.Class)}, nil
	}
	// GetNpcBotOwnerGuid(entry) is bot_ai::_ownerGuid (the owner
	// character's counter, 0 == free == bot_ai::IAmFree, bot_ai.cpp:10552)
	// for the per-bot hire menu's outer player-guid/owner gate.
	methods["GetNpcBotOwnerGuid"] = func(_ context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mgr := npcBots()
		if mgr == nil {
			return []any{uint32(0)}, nil
		}
		data, ok := mgr.Get(entry)
		if !ok {
			return []any{uint32(0)}, nil
		}
		return []any{data.Owner}, nil
	}
	// GetNpcBotCost(level, class) is BotMgr::GetNpcBotCost
	// (botmgr.cpp:1014), the hire price shown in the botgiver HIRE menu
	// labels and re-checked in the HIRE_CLASS arm.
	methods["GetNpcBotCost"] = func(_ context.Context, args []any) ([]any, error) {
		level, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		class, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		mgr := npcBots()
		if mgr == nil {
			return []any{uint64(0)}, nil
		}
		return []any{mgr.HireCost(uint8(level), uint8(class))}, nil
	}
	// RecruitNpcBot(entry) is the botgiver HIRE_ENTRY arm's paid hire:
	// NPCBotManager.Recruit == BotMgr::AddBot(bot, takeMoney=true), the
	// takeMoney=true path the gossip-hire arms always take (bot_ai.cpp:343
	// SetBotOwner pre-filters owned bots in both gossip arms, so the
	// takeMoney=false case never reaches AddBot from gossip). Returns the
	// BotAssignResult code (BotAddSuccess == 0x100 on success).
	methods["RecruitNpcBot"] = func(ctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mgr := npcBots()
		if mgr == nil {
			return []any{uint16(BotAddDisabled)}, nil
		}
		result, err := mgr.Recruit(ctx, uint32(s.playerGUID), entry)
		if err != nil {
			return nil, err
		}
		return []any{uint16(result)}, nil
	}
	methods["SetBinding"] = func(_ context.Context, _ []any) ([]any, error) { return nil, nil }
	methods["SetNotRefundable"] = func(_ context.Context, _ []any) ([]any, error) { return nil, nil }
	// IsCasting is UnitMethods::IsCasting (UnitMethods.h:546-554, Trinity):
	// HasUnitState(UNIT_STATE_CASTING). Go's cast state is s.activeCast.
	methods["IsCasting"] = func(_ context.Context, _ []any) ([]any, error) {
		s.castMu.Lock()
		casting := s.activeCast != nil
		s.castMu.Unlock()
		return []any{casting}, nil
	}
	// StopSpellCast is UnitMethods::StopSpellCast (UnitMethods.h:2532-2537):
	// Unit::CastStop (Unit.cpp:980-985) interrupts every current non-melee
	// spell except except_spellid; the Eluna arg defaults to 0.
	methods["StopSpellCast"] = func(_ context.Context, args []any) ([]any, error) {
		var spellID uint32
		if len(args) > 0 {
			id, err := luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			spellID = id
		}
		s.castMu.Lock()
		cur := s.activeCast
		s.castMu.Unlock()
		if cur != nil && (spellID == 0 || cur.SpellID != spellID) {
			s.interruptCurrentCast()
		}
		return nil, nil
	}
	// PerformEmote is UnitMethods::PerformEmote (UnitMethods.h:2041-2049):
	// Unit::HandleEmoteCommand sends SMSG_EMOTE (u32 emote + u64 guid) via
	// SendMessageToSet — self write plus visibility-set fan-out skipping self.
	methods["PerformEmote"] = func(_ context.Context, args []any) ([]any, error) {
		emote, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		pkt := protocol.NewBuffer(12)
		pkt.WriteU32(emote)
		pkt.WriteU64(s.playerGUID)
		if err := s.write(uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes(), true); err != nil {
			return nil, err
		}
		s.server.broadcastToVisibleSetFromSession(s, uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes())
		return nil, nil
	}
	// ClearThreatList is UnitMethods::ClearThreatList
	// (UnitMethods.h:1992-1999): ThreatManager::ClearAllThreat. Go has no
	// per-player threat manager, so the player leg is a no-op; the
	// creature object carries the real arm (lua_objects.go).
	methods["ClearThreatList"] = func(_ context.Context, _ []any) ([]any, error) { return nil, nil }
	// Eluna combo-point methods (PlayerMethods.h:1201-1226, 2715-2733).
	// GetComboPoints is the no-arg Unit::GetComboPoints overload
	// (Unit.h:1579): the banked count regardless of target. GetComboTarget
	// is the TRINITY arm (player->GetComboTarget()): the banked unit as a
	// Lua object, nil when nothing is banked. ClearComboPoints keeps the
	// C++ RemoveAurasByType(SPELL_AURA_RETAIN_COMBO_POINTS) arm
	// (Unit.cpp:10674-10687) that the internal session clear skips.
	methods["AddComboPoints"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("AddComboPoints requires a target and a count")
		}
		object, ok := args[0].(*scripting.Object)
		if !ok || object == nil {
			return nil, fmt.Errorf("AddComboPoints requires a unit target")
		}
		targetGUID, ok := objectUint64Field(object, "GUID")
		if !ok || targetGUID == 0 {
			return nil, fmt.Errorf("AddComboPoints requires a unit target")
		}
		count, err := luaInt64Arg(args, 1)
		if err != nil || count < math.MinInt8 || count > math.MaxInt8 {
			return nil, fmt.Errorf("AddComboPoints count must be int8")
		}
		s.addSessionComboPoints(targetGUID, int8(count))
		return nil, nil
	}
	methods["ClearComboPoints"] = func(_ context.Context, _ []any) ([]any, error) {
		s.removeAurasByType(spellAuraRetainComboPoints)
		s.clearSessionComboPoints()
		return nil, nil
	}
	// Duel-reset methods for duel_reset.cpp's DuelResetScript (the Lua
	// port subscribes to PlayerEventDuelStart/PlayerEventDuelEnd). The
	// ResetDuelCooldowns / ResetDuelHealthMana config gates live inside
	// the session methods (engine/world/duel_reset.go); the scripting
	// runtime has no world-config bridge.
	methods["SaveDuelCooldownState"] = func(_ context.Context, _ []any) ([]any, error) {
		s.saveDuelCooldownState()
		return nil, nil
	}
	methods["ResetDuelSpellCooldowns"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("ResetDuelSpellCooldowns requires the onStart flag")
		}
		onStart, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("ResetDuelSpellCooldowns requires a boolean onStart flag")
		}
		s.resetDuelSpellCooldowns(onStart)
		return nil, nil
	}
	methods["RestoreDuelCooldownState"] = func(_ context.Context, _ []any) ([]any, error) {
		s.restoreDuelCooldownState()
		return nil, nil
	}
	methods["SaveDuelHealth"] = func(_ context.Context, _ []any) ([]any, error) {
		s.saveDuelHealth()
		return nil, nil
	}
	methods["RestoreDuelHealth"] = func(_ context.Context, _ []any) ([]any, error) {
		s.restoreDuelHealth()
		return nil, nil
	}
	methods["SaveDuelMana"] = func(_ context.Context, _ []any) ([]any, error) {
		s.saveDuelMana()
		return nil, nil
	}
	methods["RestoreDuelMana"] = func(_ context.Context, _ []any) ([]any, error) {
		s.restoreDuelMana()
		return nil, nil
	}
	methods["ResetAllPowers"] = func(_ context.Context, _ []any) ([]any, error) {
		s.resetAllPowers()
		return nil, nil
	}
	methods["GetComboPoints"] = luaNoArgs(func() any { return s.sessionComboPoints(0) })
	methods["GetComboTarget"] = func(ctx context.Context, _ []any) ([]any, error) {
		if s.comboTargetGUID == 0 {
			return []any{nil}, nil
		}
		if player := s.server.findPlayer(s.comboTargetGUID); player != nil {
			return []any{player}, nil
		}
		if creature := s.luaCreature(ctx, s.comboTargetGUID); creature != nil {
			return []any{creature}, nil
		}
		return []any{nil}, nil
	}
	// Eluna session/account methods (PlayerMethods.h:862-866, 1495-1519,
	// 1550-1579). GetLatency is WorldSession::GetLatency in ms; the account
	// id/name are the login values; GetPlayerIP is
	// WorldSession::GetRemoteAddress (no port).
	methods["GetLatency"] = luaNoArgs(func() any { return s.latency.Load() })
	methods["GetAccountId"] = luaNoArgs(func() any { return s.accountID })
	methods["GetAccountName"] = luaNoArgs(func() any { return s.accountName })
	methods["GetPlayerIP"] = luaNoArgs(func() any { return s.remoteIP() })
	// Eluna time/talent/reputation/guild methods (PlayerMethods.h:1002,
	// 1155-1167, 1169-1199, 1508-1516, 1227-1232). GetLevelPlayedTime is
	// flushed like Player::Update's m_Played_time accumulation so the read
	// is current. GetInGameTime returns the login-time server-uptime value
	// (CharacterHandler.cpp:856), not played time. GetFreeTalentPoints is
	// the PLAYER_CHARACTER_POINTS1 twin. GetGuildName pushes nil for the
	// guildless, like the C++ early return.
	methods["GetLevelPlayedTime"] = luaNoArgs(func() any {
		s.updatePlayedTime(time.Now())
		return state.LevelPlayedTime
	})
	methods["GetInGameTime"] = luaNoArgs(func() any { return s.loginGameTimeMS })
	methods["GetFreeTalentPoints"] = luaNoArgs(func() any { return freeTalentPoints(state) })
	// Eluna Player talent methods (PlayerMethods.h:32-48, 2107-2116,
	// 2653-2686, 3705-3719; Player.cpp:3957-4087, 4171-4175, 9612-9630,
	// 25460-25559). HasTalent resolves the spell to its talent rank via
	// TalentBySpell and compares against the learned rank for that spec:
	// the C++ map is keyed by spell, so a different rank of the same
	// talent reads false. Spec >= 2 (MAX_TALENT_SPECS) reads nil, the
	// Eluna early return. The active spec reads the in-memory map; any
	// other spec reads its stored rows via loadTalentsForGroup.
	// LearnTalent runs the shared learn arm and always resends the
	// talent frame, then fires event 39 on success (the C++ learn path
	// fires OnLearnTalents only after a successful learn). ResetTalents
	// defaults no_cost to true like CHECKVAL<bool>(L, 2, true); event 17
	// fires on entry (ScriptMgr::OnPlayerTalentsReset runs before the
	// no-talents early return), and the frame is resent exactly once:
	// resetTalents already sends on success, so the binding sends only
	// on its false paths. ResetTalentsCost reads the escalating ladder
	// (resetTalentsCost). ResetPetTalents runs the shared hunter-pet arm
	// and resends the pet frame exactly once the same way.
	// SetFreeTalentPoints is Player::SetFreeTalentPoints
	// (Player.cpp:4087): the PLAYER_CHARACTER_POINTS1 write plus the
	// talents-frame resend, with the OnFreeTalentPointsChanged hook
	// (event 16) fired from the set itself. Go stores the value as a
	// session-lifetime override honored by both freeTalentPoints
	// twins; learn decrements it by the gated cost, reset and
	// level-up clear it (Player.cpp:4064, GiveLevel:2745), and login
	// recomputes from level minus spent (InitTalentForLevel), so it
	// does not persist.
	methods["SetFreeTalentPoints"] = func(ctx context.Context, args []any) ([]any, error) {
		points, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		state.FreeTalentPointsOverride = points
		state.FreeTalentPointsOverrideSet = true
		s.triggerPlayerEvent(ctx, scripting.PlayerEventTalentChange, s.luaPlayer(), points)
		_ = s.sendTalentsInfo(false)
		return []any{}, nil
	}
	methods["HasTalent"] = func(_ context.Context, args []any) ([]any, error) {
		spellID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		spec, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if spec >= 2 {
			return []any{nil}, nil
		}
		if s.server == nil || s.server.Data == nil {
			return []any{false}, nil
		}
		talentID, rank, ok := s.server.Data.TalentBySpell(spellID)
		if !ok {
			return []any{false}, nil
		}
		talents := state.Talents
		if uint8(spec) != state.ActiveTalentGroup {
			talents = s.loadTalentsForGroup(uint8(spec))
		}
		learned, has := talents[talentID]
		return []any{has && learned == rank}, nil
	}
	methods["LearnTalent"] = func(ctx context.Context, args []any) ([]any, error) {
		talentID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		rank, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		learned := s.learnTalent(ctx, talentID, rank)
		_ = s.sendTalentsInfo(false)
		if learned && s.server != nil && s.server.Data != nil {
			if tEntry, ok, terr := s.server.Data.Talent(talentID); terr == nil && ok && rank < uint32(len(tEntry.SpellRank)) {
				s.triggerPlayerEvent(ctx, scripting.PlayerEventLearnTalents, s.luaPlayer(), talentID, rank, tEntry.SpellRank[rank])
			}
		}
		return nil, nil
	}
	methods["ResetTalents"] = func(ctx context.Context, args []any) ([]any, error) {
		noCost := true
		if len(args) > 0 {
			if b, ok := args[0].(bool); ok {
				noCost = b
			}
		}
		s.triggerPlayerEvent(ctx, scripting.PlayerEventTalentReset, s.luaPlayer(), noCost)
		if !s.resetTalents(ctx, noCost) {
			_ = s.sendTalentsInfo(false)
		}
		return nil, nil
	}
	methods["ResetTalentsCost"] = luaNoArgs(func() any { return s.resetTalentsCost() })
	methods["ResetPetTalents"] = func(ctx context.Context, _ []any) ([]any, error) {
		if !s.resetActivePetTalents(ctx) {
			_ = s.sendTalentsInfo(true)
		}
		return nil, nil
	}
	// Eluna Player achievement methods (PlayerMethods.h:55-64, 2068-2076,
	// 2124-2131; Player.cpp:25430-25432, 25455-25458; AchievementMgr.cpp:486-508,
	// 1629-1632). HasAchieved is a plain earned-map lookup with no DBC gate
	// (C++ HasAchieved reads m_completedAchievements only). SetAchievement
	// no-ops on a DBC miss (the C++ LookupEntry guard) and otherwise runs
	// the shared complete arm, which itself no-ops on already-earned and
	// counter achievements; the DBC gate also keeps a nil-server session
	// from reaching completeAchievement's server dereference.
	// ResetAchievements runs the shared reset arm; its CheckAll re-fill leg
	// stays out (see resetAchievements).
	methods["HasAchieved"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		_, has := s.earnedAchievements[id]
		return []any{has}, nil
	}
	methods["SetAchievement"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s.knownAchievement(id) {
			s.completeAchievement(id)
		}
		return nil, nil
	}
	methods["ResetAchievements"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.resetAchievements(ctx)
		return nil, nil
	}
	methods["GetReputation"] = func(_ context.Context, args []any) ([]any, error) {
		faction, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{s.reputationStanding(faction)}, nil
	}
	methods["GetGuildName"] = func(ctx context.Context, _ []any) ([]any, error) {
		if state.GuildID == 0 {
			return []any{nil}, nil
		}
		return []any{s.server.getGuildName(ctx, state.GuildID)}, nil
	}
	// Eluna Player:GetGuild/GetGroup (PlayerMethods.h:1535/1546). GetGuild
	// pushes the player's live Guild object, nil when guildless like
	// Eluna::Push((Guild*)nullptr); GetGroup pushes the live Group object,
	// nil when the player is not in a group.
	methods["GetGuild"] = func(ctx context.Context, _ []any) ([]any, error) {
		var out any
		if guild := s.luaGuildObject(ctx, state.GuildID); guild != nil {
			out = guild
		}
		return []any{out}, nil
	}
	methods["GetGroup"] = func(_ context.Context, _ []any) ([]any, error) {
		var out any
		if s.groupID != 0 {
			if group := groupLuaObject(s.server, s.server.findGroupByID(s.groupID)); group != nil {
				out = group
			}
		}
		return []any{out}, nil
	}
	// GetGroupInvite is Player::GetGroupInvite (PlayerMethods.h:933-937;
	// Player.h:2083): the Group the player is invited to, nil when no
	// invite is pending. Go stores the pending invite as the inviter's
	// GUID (pendingGroupLeader), not a Group object, so the group is
	// resolved the way the accept path does (group.go): the inviter's
	// live group, else a group the inviter leads (a zoning/offline
	// leader's created group), else nil. An invite from a groupless
	// inviter has no Go group yet — C++ materializes an uncreated
	// Group there, Go creates it only on accept — so it reads nil.
	methods["GetGroupInvite"] = func(_ context.Context, _ []any) ([]any, error) {
		var out any
		if s.pendingGroupLeader != 0 && s.server != nil {
			var g *groupState
			if leader := s.server.findSessionByGUID(s.pendingGroupLeader); leader != nil && leader.groupID != 0 {
				g = s.server.findGroupByID(leader.groupID)
			}
			if g == nil {
				s.server.groupsMu.RLock()
				for _, grp := range s.server.groups {
					if grp.LeaderGUID == s.pendingGroupLeader {
						g = grp
						break
					}
				}
				s.server.groupsMu.RUnlock()
			}
			if group := groupLuaObject(s.server, g); group != nil {
				out = group
			}
		}
		return []any{out}, nil
	}
	// luaGroupInviteTarget resolves a Player object argument to its live
	// session, the CHECKOBJ<Player> twin shared by GroupInvite and
	// GroupCreate: only a live session can hold a group or a pending
	// invite, so an unresolvable argument errors like the sibling
	// player-argument methods instead of reading a nil group.
	luaGroupInviteTarget := func(args []any) (*session, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("player is required")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("player is required")
		}
		guid, ok := objectUint64Field(obj, "GUID")
		if !ok || guid == 0 || s.server == nil {
			return nil, fmt.Errorf("player is required")
		}
		invited := s.server.findSessionByGUID(guid)
		if invited == nil || invited.player == nil {
			return nil, fmt.Errorf("player is required")
		}
		return invited, nil
	}
	// luaInviteGroup resolves the group an invite goes into (the
	// GroupInvite arm of PlayerMethods.h:3968-3972): the caller's group,
	// with a BG group substituted by the original group stashed by
	// SetBattlegroundOrBattlefieldRaid, or nil when the caller is
	// groupless or rides a BG raid with no original.
	luaInviteGroup := func() *groupState {
		if s.groupID == 0 || s.server == nil {
			return nil
		}
		g := s.server.findGroupByID(s.groupID)
		if g != nil && g.isBGGroup() {
			if s.originalGroupID == 0 {
				return nil
			}
			return s.server.findGroupByID(s.originalGroupID)
		}
		return g
	}
	// GetNextRandomRaidMember is Player::GetNextRandomRaidMember
	// (PlayerMethods.h:909-916; Player.cpp:24394-24417): a random group
	// member other than the caller, within radius by IsWithinDistInMap
	// (Object.cpp:1238-1240: same map, phase overlap, 3D distance plus
	// both combat reaches), skipping invisibility-aura holders and
	// hostiles (the C++ comment's duel arm included: a duel opponent
	// with both DuelTeam values set reads hostile; controlled-by-enemy
	// has no Go member model, members are players only). Nil when
	// ungrouped or when no member passes. Go's roster is online-only,
	// so offline slots never resolve, matching the C++ map iteration.
	methods["GetNextRandomRaidMember"] = func(_ context.Context, args []any) ([]any, error) {
		radius, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s.groupID == 0 || s.server == nil || s.player == nil {
			return []any{nil}, nil
		}
		group := s.server.findGroupByID(s.groupID)
		if group == nil {
			return []any{nil}, nil
		}
		s.server.groupsMu.RLock()
		memberGUIDs := make([]uint64, 0, len(group.Members))
		for _, m := range group.Members {
			memberGUIDs = append(memberGUIDs, m.GUID)
		}
		s.server.groupsMu.RUnlock()
		ownReach := float64(s.player.CombatReach)
		if ownReach <= 0 {
			ownReach = 1.5
		}
		ownPhase := luaRangePhase(s.currentPlayerPhaseMask())
		var near []*session
		for _, guid := range memberGUIDs {
			if guid == s.playerGUID {
				continue
			}
			cand := s.server.findSessionByGUID(guid)
			if cand == nil || cand.player == nil {
				continue
			}
			if cand.player.Map != s.player.Map || cand.player.InstanceID != s.player.InstanceID {
				continue
			}
			if luaRangePhase(cand.currentPlayerPhaseMask())&ownPhase == 0 {
				continue
			}
			candReach := float64(cand.player.CombatReach)
			if candReach <= 0 {
				candReach = 1.5
			}
			dx := float64(cand.player.X - s.player.X)
			dy := float64(cand.player.Y - s.player.Y)
			dz := float64(cand.player.Z - s.player.Z)
			if math.Sqrt(dx*dx+dy*dy+dz*dz) > float64(radius)+ownReach+candReach {
				continue
			}
			if cand.hasAuraType(spellAuraInvisibility) {
				continue
			}
			if s.server.playerHostileToUnit(s, cand.effectiveFactionTemplate(cand.player.Race)) {
				continue
			}
			if s.duelPartner == guid && s.player.DuelTeam != 0 && cand.player.DuelTeam != 0 {
				continue
			}
			near = append(near, cand)
		}
		if len(near) == 0 {
			return []any{nil}, nil
		}
		return []any{luaPlayerOrNil(near[rand.Intn(len(near))])}, nil
	}
	// GroupInvite is Player::GroupInvite (PlayerMethods.h:3948-4010):
	// false when the invited player already holds a group or a pending
	// invite (the Lua pre-check, stricter than Group::AddInvite's BG
	// arm); otherwise an invite into the caller's group when one
	// resolves (full groups refuse, like Group::IsFull), or a leader
	// invite for a new group when none does (which fails while the
	// caller holds a pending invite, the AddLeaderInvite arm). Success
	// sends SMSG_GROUP_INVITE to the invited player and fires
	// GROUP_EVENT_ON_MEMBER_INVITE: once for the invitee with an
	// existing group, twice for a groupless inviter (leader, then
	// invitee, over the packet path's ephemeral group object, whose
	// leader the C++ AddLeaderInvite arm supplies). The ephemeral
	// object is event-only: GetGroupInvite still reads nil for a
	// groupless inviter, since no group is stored until accept.
	methods["GroupInvite"] = func(ctx context.Context, args []any) ([]any, error) {
		invited, err := luaGroupInviteTarget(args)
		if err != nil {
			return nil, err
		}
		if invited.groupID != 0 || invited.pendingGroupLeader != 0 {
			return []any{false}, nil
		}
		group := luaInviteGroup()
		if group != nil {
			if s.server.groupSlotFull(ctx, group) {
				return []any{false}, nil
			}
			s.server.groupsMu.Lock()
			invited.pendingGroupLeader = s.playerGUID
			groupObj := groupLuaObject(s.server, group)
			s.server.groupsMu.Unlock()
			_ = invited.write(uint16(protocol.OpcodeSMSG_GROUP_INVITE), buildGroupInvite(1, s.player.Name), true)
			s.server.triggerGroupEvent(scripting.GroupEventOnMemberInvite, groupObj, invited.playerGUID)
			return []any{true}, nil
		}
		if s.pendingGroupLeader != 0 {
			return []any{false}, nil
		}
		invited.pendingGroupLeader = s.playerGUID
		ephemeral := scripting.NewGroupObject(groupGUID(0), scripting.GroupLive{
			Load: func() (scripting.GroupSnapshot, bool) {
				return scripting.GroupSnapshot{
					LeaderGUID: s.playerGUID,
					Members:    []scripting.GroupMemberLive{{GUID: s.playerGUID}},
				}, true
			},
			Players: s.server.luaPlayers,
		})
		s.server.triggerGroupEvent(scripting.GroupEventOnMemberInvite, ephemeral, s.playerGUID)
		s.server.triggerGroupEvent(scripting.GroupEventOnMemberInvite, ephemeral, invited.playerGUID)
		if s.server.Features != nil && s.server.Features.LFG != nil &&
			s.server.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) != 0 {
			if s.server.Features.LFG.Leave(s.playerGUID) {
				_ = s.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: s.playerGUID, State: LFGStateNone})
			} else if prop := s.server.Features.LFG.DeclineProposalForPlayer(s.playerGUID); prop != nil {
				s.server.sendLFGProposalDeclined(prop)
			}
		}
		_ = invited.write(uint16(protocol.OpcodeSMSG_GROUP_INVITE), buildGroupInvite(1, s.player.Name), true)
		return []any{true}, nil
	}
	// GroupCreate is Player::GroupCreate (PlayerMethods.h:4012-4063):
	// nil when the caller or the invited player already holds a group;
	// each side's pending invite is withdrawn first (UninviteFromGroup,
	// whose created-group disband arm has no Go target: a pending
	// invite into a created group leaves that group's member count
	// untouched). The group is created at once, the accept path's
	// creation shape (leader difficulties, group-loot defaults, DB rows
	// when a store is present, leader flag), then the invited player
	// joins as a party member with a list broadcast, the LFG-leave and
	// member-add event of the accept path's AddMember arm.
	methods["GroupCreate"] = func(ctx context.Context, args []any) ([]any, error) {
		invited, err := luaGroupInviteTarget(args)
		if err != nil {
			return nil, err
		}
		if s.groupID != 0 || invited.groupID != 0 {
			return []any{nil}, nil
		}
		srv := s.server
		srv.groupsMu.Lock()
		s.pendingGroupLeader = 0
		invited.pendingGroupLeader = 0
		g := &groupState{
			ID:            newGroupID(),
			LeaderGUID:    s.playerGUID,
			LootMethod:    3,
			LootThreshold: 2,
			LooterGUID:    s.playerGUID,
			DungeonDiff:   s.player.DungeonDifficulty,
			RaidDiff:      s.player.RaidDifficulty,
		}
		g.Members = append(g.Members, groupMember{GUID: s.playerGUID, Name: s.player.Name})
		refreshGroupMaxEnchantingLevel(srv, g)
		srv.groups[g.ID] = g
		s.groupID = g.ID
		if srv.CharactersStore != nil && srv.CharactersStore.DB != nil {
			var nextID int64
			if err := srv.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM `groups`").Scan(&nextID); err == nil && nextID > 0 {
				if _, err := srv.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_INS_GROUP"), uint32(nextID), s.playerGUID, g.LootMethod, g.LooterGUID, g.LootThreshold, 0, 0, 0, 0, 0, 0, 0, 0, g.GroupType, g.DungeonDiff, g.RaidDiff, g.MasterLooter); err == nil {
					g.DBID = uint32(nextID)
					_, _ = srv.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_INS_GROUP_MEMBER"), g.DBID, s.playerGUID, 0, 0, 0)
				}
			}
		}
		g.Members = append(g.Members, groupMember{GUID: invited.playerGUID, Name: invited.player.Name})
		refreshGroupMaxEnchantingLevel(srv, g)
		invited.groupID = g.ID
		groupObj := groupLuaObject(srv, g)
		srv.groupsMu.Unlock()
		if g.DBID != 0 && srv.CharactersStore != nil && srv.CharactersStore.DB != nil {
			_, _ = srv.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_INS_GROUP_MEMBER"), g.DBID, invited.playerGUID, 0, 0, 0)
		}
		s.player.PlayerFlags = UpdatePlayerGroupLeaderFlag(s.player.PlayerFlags, true)
		s.sendPlayerUpdate()
		invited.instanceValid = invited.checkInstanceValidity(ctx, g)
		if srv.Features != nil && srv.Features.LFG != nil && srv.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) != 0 && srv.Features.LFG.Leave(invited.playerGUID) {
			_ = invited.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: invited.playerGUID, State: LFGStateNone})
		}
		srv.broadcastGroupList(g)
		now := time.Now()
		invited.groupStatsSnapshot = invited.currentGroupStatsChanged(ctx)
		invited.groupStatsInit = true
		invited.groupStatsDue = now.Add(5 * time.Second)
		full := invited.groupStatsSnapshot
		full.UpdateFlags = protocol.GroupUpdateFullMask
		srv.sendGroupStatsToOutOfRange(invited, protocol.BuildPartyMemberStatsChanged(full))
		srv.triggerGroupEvent(scripting.GroupEventOnMemberAdd, groupObj, invited.playerGUID)
		return []any{groupObj}, nil
	}
	// RemoveFromGroup is Player::RemoveFromGroup (PlayerMethods.h:2639-2647;
	// Player.h:1555): the no-group arm is a silent no-op, otherwise the
	// default-method removal shared with the GM .group remove command,
	// including its disband tail and member-remove event.
	methods["RemoveFromGroup"] = func(ctx context.Context, _ []any) ([]any, error) {
		if s.groupID == 0 || s.server == nil {
			return []any{}, nil
		}
		group := s.server.findGroupByID(s.groupID)
		if group == nil {
			return []any{}, nil
		}
		s.removeFromGroup(ctx, group, s, groupRemoveMethodDefault)
		return []any{}, nil
	}
	// GetSubGroup is Player::GetSubGroup (PlayerMethods.h:922-926;
	// Player.h:2089): the member's subgroup in the session group,
	// 0 when ungrouped or not a member (the Group::GetMemberGroup
	// miss arm).
	methods["GetSubGroup"] = luaNoArgs(func() any {
		if s.groupID == 0 || s.server == nil {
			return uint8(0)
		}
		group := s.server.findGroupByID(s.groupID)
		if group == nil {
			return uint8(0)
		}
		sub, ok := group.memberSubGroup(s.playerGUID)
		if !ok {
			return uint8(0)
		}
		return sub
	})
	// GetOriginalGroup/GetOriginalSubGroup are Player::GetOriginalGroup/
	// GetOriginalSubGroup (PlayerMethods.h:886-900; Player.h:2100-2102):
	// the group stashed by SetBattlegroundOrBattlefieldRaid while the
	// player rides a BG raid, and its subgroup. Go keeps them as
	// originalGroupID/originalSubGroup (battleground_group.go); both
	// read nil/0 outside a BG raid, and the restore path clears them.
	methods["GetOriginalGroup"] = func(_ context.Context, _ []any) ([]any, error) {
		var out any
		if s.originalGroupID != 0 && s.server != nil {
			if group := groupLuaObject(s.server, s.server.findGroupByID(s.originalGroupID)); group != nil {
				out = group
			}
		}
		return []any{out}, nil
	}
	methods["GetOriginalSubGroup"] = luaNoArgs(func() any { return s.originalSubGroup })
	// IsInSameRaidWith is Player::IsInSameRaidWith (Player.cpp:2543):
	// the same player, or two players in the same non-nil group. The
	// Go twin is sameRaidGroup (gameobjects.go): Go has no raid/party
	// distinction, so a shared non-zero groupID is the raid test.
	// Backs the soulwell gossip gate (go_scripts.cpp:538-564).
	methods["IsInSameRaidWith"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("player is required")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("player is required")
		}
		otherGUID, ok := objectUint64Field(obj, "GUID")
		if !ok || otherGUID == 0 {
			return []any{false}, nil
		}
		if otherGUID == state.GUID {
			return []any{true}, nil
		}
		other := s.server.findSessionByGUID(otherGUID)
		return []any{sameRaidGroup(s, other)}, nil
	}
	// IsInSameGroupWith is Player::IsInSameGroupWith (PlayerMethods.h:555-559;
	// Player.cpp:2536-2540): the same player, or two players in the same
	// group in the same subgroup (Group::SameSubGroup). Go has no raid/
	// party distinction, so the subgroup equality is the whole delta
	// from IsInSameRaidWith above.
	methods["IsInSameGroupWith"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("player is required")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("player is required")
		}
		otherGUID, ok := objectUint64Field(obj, "GUID")
		if !ok || otherGUID == 0 {
			return []any{false}, nil
		}
		if otherGUID == state.GUID {
			return []any{true}, nil
		}
		other := s.server.findSessionByGUID(otherGUID)
		if other == nil || s.groupID == 0 || s.groupID != other.groupID {
			return []any{false}, nil
		}
		group := s.server.findGroupByID(s.groupID)
		if group == nil {
			return []any{false}, nil
		}
		sub, ok := group.memberSubGroup(s.playerGUID)
		if !ok {
			return []any{false}, nil
		}
		otherSub, ok := group.memberSubGroup(otherGUID)
		return []any{ok && sub == otherSub}, nil
	}
	// IsGroupVisibleFor is Player::IsGroupVisibleFor (Player.cpp:2525-2535):
	// a CONFIG_GROUP_VISIBILITY switch whose default (0) leg is
	// IsInSameGroupWith above, so the Go twin repeats that leg verbatim
	// (same player, same group, same subgroup). Go has no group-visibility
	// config surface, so modes 1 (same raid — the bound IsInSameRaidWith),
	// 2 (same team) and 3 (never visible) stay unmodeled; a missing or
	// non-object argument errors like Eluna's CHECKOBJ, and a GUID with
	// no live session reads false.
	methods["IsGroupVisibleFor"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("player is required")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("player is required")
		}
		otherGUID, ok := objectUint64Field(obj, "GUID")
		if !ok || otherGUID == 0 {
			return []any{false}, nil
		}
		if otherGUID == state.GUID {
			return []any{true}, nil
		}
		other := s.server.findSessionByGUID(otherGUID)
		if other == nil || s.groupID == 0 || s.groupID != other.groupID {
			return []any{false}, nil
		}
		group := s.server.findGroupByID(s.groupID)
		if group == nil {
			return []any{false}, nil
		}
		sub, ok := group.memberSubGroup(s.playerGUID)
		if !ok {
			return []any{false}, nil
		}
		otherSub, ok := group.memberSubGroup(otherGUID)
		return []any{ok && sub == otherSub}, nil
	}
	// IsVisibleForPlayer is Player::IsVisibleGloballyFor
	// (PlayerMethods.h:582-588, Player.cpp:22599): note the direction —
	// C++ asks whether the CALLER is visible to the argument player, not
	// whether the caller sees the argument, so the Go twin runs with
	// the argument as viewer and the caller as target (social.go). A
	// missing or non-object argument errors like Eluna's CHECKOBJ; a
	// GUID with no live session reads false, and the caller's own GUID
	// reads true without a lookup, matching the twin's self arm.
	methods["IsVisibleForPlayer"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("player is required")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("player is required")
		}
		otherGUID, ok := objectUint64Field(obj, "GUID")
		if !ok || otherGUID == 0 {
			return []any{false}, nil
		}
		if otherGUID == state.GUID {
			return []any{true}, nil
		}
		other := s.server.findSessionByGUID(otherGUID)
		if other == nil {
			return []any{false}, nil
		}
		return []any{isVisibleGloballyFor(other, s)}, nil
	}
	// IsHonorOrXPTarget is Player::isHonorOrXPTarget
	// (PlayerMethods.h:568-574, Player.cpp:24064-24083): the victim
	// level must clear the caller's gray level — the scaled-XP-ratio
	// escape has no Go twin (MinDiscoveredScaledXPRatio is a different
	// config), so the gate applies unconditionally, the same reading
	// the death path uses. Creature victims additionally fail as live
	// totems (isTotemGUID) or critters (creature_template.type 8, the
	// live-motion CreatureType twin); player victims take the gray
	// gate only, and the NPCBot arm has no Go counterpart. A missing
	// or non-unit argument errors like Eluna's CHECKOBJ<Unit>.
	methods["IsHonorOrXPTarget"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("unit is required")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("unit is required")
		}
		if obj.Type != "Player" && obj.Type != "Creature" {
			return nil, fmt.Errorf("unit is required")
		}
		victimLevel, ok := objectUint32Field(obj, "Level")
		if !ok {
			return []any{false}, nil
		}
		if victimLevel <= grayLevel(uint32(state.Level)) {
			return []any{false}, nil
		}
		if obj.Type == "Creature" {
			guid, _ := objectUint64Field(obj, "GUID")
			if s.server != nil && s.server.isTotemGUID(guid) {
				return []any{false}, nil
			}
			entry, hasEntry := objectUint32Field(obj, "Entry")
			if hasEntry && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
				var creatureType int64
				if err := s.server.WorldStore.DB.QueryRow("SELECT COALESCE(type, 0) FROM creature_template WHERE entry = ?", entry).Scan(&creatureType); err == nil && uint32(creatureType) == creatureTypeCritter {
					return []any{false}, nil
				}
			}
		}
		return []any{true}, nil
	}
	// Eluna WorldObject sound methods (WorldObjectMethods.h:1161-1242) —
	// replaces the old no-op PlayDirectSound stub.
	luaWorldObjectSoundMethods(s.server, s, s.playerGUID, methods)
	// SetVisibleItemEnchantment ports the visual-only enchant write of the
	// custom weapon-visual scripts
	// (src/server/scripts/Custom/cs_npc_weapon_visual.cpp,
	// TrintyRetardsAI::SetVisual, and the denveous_stuff item script):
	// player->SetUInt16Value(PLAYER_VISIBLE_ITEM_1_ENCHANTMENT + (slot * 2),
	// 0, visual). The low uint16 of the enchantment update word is the
	// permanent enchant the client renders as the weapon glow; the high
	// uint16 (temporary enchant) is preserved, matching SetUInt16Value's
	// word write. slot is the equipment slot (EQUIPMENT_SLOT_MAINHAND = 15,
	// EQUIPMENT_SLOT_OFFHAND = 16); the update is ephemeral like the C++
	// field write — a relog rebuilds the word from item_instance
	// enchantments. Returns true when the visual was applied.
	methods["SetVisibleItemEnchantment"] = func(lctx context.Context, args []any) ([]any, error) {
		if s.player == nil {
			return []any{false}, nil
		}
		slot, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		visual, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if slot >= uint32(playerVisibleItemCount) {
			return []any{false}, nil
		}
		pairs := strings.Fields(s.player.Equipment)
		base := int(slot) * 2
		var itemID, enchant uint64
		if base < len(pairs) {
			itemID, _ = strconv.ParseUint(pairs[base], 10, 32)
			if base+1 < len(pairs) {
				enchant, _ = strconv.ParseUint(pairs[base+1], 10, 32)
			}
		}
		if itemID == 0 {
			// C++: ChatHandler(player->GetSession()).PSendSysMessage("Equip
			// an item in that slot in order to change the look.")
			_ = s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), protocol.BuildSystemChatMessage("Equip an item in that slot in order to change the look."), true)
			return []any{false}, nil
		}
		// C++ SetVisual subclass gate, a raw SubClass compare with no class
		// check (ItemTemplate.h:349-369, 400-401) — so armor 5/6 reject the
		// same way as weapon polearm/2h-mace 5/6:
		// shield 6, buckler 5, bow 2, gun 3, obsolete 9, exotic 11,
		// exotic2 12, misc 14, thrown 16, spear 17, crossbow 18, wand 19,
		// fishing pole 20.
		if data, dataErr := s.loadItemQueryData(lctx, uint32(itemID)); dataErr == nil {
			switch data.SubClass {
			case 2, 3, 5, 6, 9, 11, 12, 14, 16, 17, 18, 19, 20:
				return []any{false}, nil
			}
		}
		newEnchant := (uint32(enchant) & 0xFFFF0000) | (uint32(visual) & 0xFFFF)
		if newEnchant != uint32(enchant) {
			padded := make([]string, base+2)
			copy(padded, pairs)
			for i := len(pairs); i < base+2; i++ {
				padded[i] = "0"
			}
			padded[base] = strconv.FormatUint(itemID, 10)
			padded[base+1] = strconv.FormatUint(uint64(newEnchant), 10)
			s.player.Equipment = strings.Join(padded, " ") + " "
		}
		fields := map[int]uint32{
			int(playerVisibleItemStart) + int(slot)*2:     uint32(itemID),
			int(playerVisibleItemStart) + int(slot)*2 + 1: newEnchant,
		}
		if packet, perr := s.server.buildPlayerValuesUpdate(s.playerGUID, fields); perr == nil && packet != nil {
			_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		}
		s.server.broadcastPlayerValuesUpdateFromSession(s, fields)
		return []any{true}, nil
	}
	methods["GetItemByPos"] = func(_ context.Context, _ []any) ([]any, error) { return []any{nil}, nil }
	// GetItemByEntry/GetItemByGUID/GetEquippedItemBySlot are
	// Player::GetItemByEntry/GetItemByGuid/GetEquippedItemBySlot
	// (PlayerMethods.h:1301-1305, 1376-1403; Player.cpp:9994-10016,
	// 12979-13006). ByEntry scans inventory only — backpack, keyring,
	// equipped-bag contents, then equipment + equipped bags — and
	// never bank, despite the Eluna doc claiming otherwise; ByGuid
	// scans inventory + bank and needs no order (GUIDs are unique);
	// BySlot reads bag-0 slot < EQUIPMENT_SLOT_END (19), wider
	// slots read nil. All three resolve through sessionLuaItem, so
	// the returned object carries the instance/template surface;
	// a missing store, missing row, or query error reads nil, the
	// sessionLuaItem convention (the GetItemByPos stub stays as-is).
	methods["GetItemByEntry"] = func(ctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return []any{nil}, nil
		}
		var item int64
		err = s.server.CharactersStore.DB.QueryRowContext(ctx,
			`SELECT ci.item FROM character_inventory ci
			 JOIN item_instance ii ON ii.guid = ci.item
			 LEFT JOIN character_inventory cb ON cb.guid = ? AND cb.bag = 0 AND cb.slot >= 19 AND cb.slot < 23 AND cb.item != 0 AND cb.item = ci.bag
			 WHERE ci.guid = ? AND ii.itemEntry = ? AND ci.item != 0
			 AND ((ci.bag = 0 AND (ci.slot < 39 OR (ci.slot >= 86 AND ci.slot < 150))) OR (ci.bag != 0 AND cb.item IS NOT NULL))
			 ORDER BY CASE
			 WHEN ci.bag = 0 AND ci.slot >= 23 AND ci.slot < 39 THEN 0
			 WHEN ci.bag = 0 AND ci.slot >= 86 AND ci.slot < 150 THEN 1
			 WHEN ci.bag != 0 THEN 2
			 ELSE 3 END, COALESCE(cb.slot, 0), ci.slot LIMIT 1`,
			state.GUID, state.GUID, entry).Scan(&item)
		if err != nil || item == 0 {
			return []any{nil}, nil
		}
		if obj := s.sessionLuaItem(ctx, uint64(item)); obj != nil {
			return []any{obj}, nil
		}
		return []any{nil}, nil
	}
	methods["GetItemByGUID"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return []any{nil}, nil
		}
		high := uint64(0x4000) << 48
		var item int64
		err = s.server.CharactersStore.DB.QueryRowContext(ctx,
			`SELECT item FROM character_inventory WHERE guid = ? AND item != 0 AND (item = ? OR item = ? OR item = ?) LIMIT 1`,
			state.GUID, guid, guid|high, guid&^high).Scan(&item)
		if err != nil || item == 0 {
			return []any{nil}, nil
		}
		if obj := s.sessionLuaItem(ctx, uint64(item)); obj != nil {
			return []any{obj}, nil
		}
		return []any{nil}, nil
	}
	methods["GetEquippedItemBySlot"] = func(ctx context.Context, args []any) ([]any, error) {
		slotValue, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		slot := uint8(slotValue)
		if slot >= 19 {
			return []any{nil}, nil
		}
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return []any{nil}, nil
		}
		var item int64
		err = s.server.CharactersStore.DB.QueryRowContext(ctx,
			`SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? AND item != 0 LIMIT 1`,
			state.GUID, slot).Scan(&item)
		if err != nil || item == 0 {
			return []any{nil}, nil
		}
		if obj := s.sessionLuaItem(ctx, uint64(item)); obj != nil {
			return []any{obj}, nil
		}
		return []any{nil}, nil
	}
	// DurabilityLoss/DurabilityPointsLoss/DurabilityLossAll/
	// DurabilityPointsLossAll/DurabilityPointLossForEquipSlot/
	// DurabilityRepair/DurabilityRepairAll are Player::Durability*
	// (PlayerMethods.h:2514-2625; Player.cpp:4890-5101) over the
	// items.go durability model: the batch pairs reuse
	// durabilityLossAll/durabilityPointsLossAll, the single-item
	// pair uses durabilityLoss/durabilityPointsLoss, the equip-slot
	// loss is a 1-point loss on the bag-0 slot item (out-of-range
	// slots are a silent no-op, the binding's gate), and the
	// repairs use durabilityRepairByPos/durabilityRepairAll with
	// the C++ (bag<<8)|slot position encoding. Zero live Lua
	// callers — engine-correctness. Arg notes: DurabilityRepair's
	// discountMod has no C++ default (CHECKVAL without one), so a
	// missing value errors like other required Go args, while
	// RepairAll's discountMod defaults to 1 per its binding;
	// guildBank defaults false on both.
	methods["DurabilityLoss"] = func(ctx context.Context, args []any) ([]any, error) {
		itemGUID, err := luaItemGUIDArg(args, 0)
		if err != nil {
			return nil, err
		}
		percent, err := luaGenericFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if itemGUID != 0 {
			s.durabilityLoss(ctx, itemGUID, float64(percent))
		}
		return nil, nil
	}
	methods["DurabilityPointsLoss"] = func(ctx context.Context, args []any) ([]any, error) {
		itemGUID, err := luaItemGUIDArg(args, 0)
		if err != nil {
			return nil, err
		}
		points, err := luaInt32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if itemGUID != 0 {
			s.durabilityPointsLoss(ctx, itemGUID, points)
		}
		return nil, nil
	}
	methods["DurabilityLossAll"] = func(ctx context.Context, args []any) ([]any, error) {
		percent, err := luaGenericFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		inventory := true
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				inventory = b
			}
		}
		s.durabilityLossAll(ctx, float64(percent), inventory)
		return nil, nil
	}
	methods["DurabilityPointsLossAll"] = func(ctx context.Context, args []any) ([]any, error) {
		points, err := luaInt32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		inventory := true
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				inventory = b
			}
		}
		s.durabilityPointsLossAll(ctx, points, inventory)
		return nil, nil
	}
	methods["DurabilityPointLossForEquipSlot"] = func(ctx context.Context, args []any) ([]any, error) {
		slotValue, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if slotValue >= uint32(equipSlotEnd) {
			return nil, nil
		}
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return nil, nil
		}
		var itemGUID int64
		err = s.server.CharactersStore.DB.QueryRowContext(ctx,
			`SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? AND item != 0 LIMIT 1`,
			s.playerGUID, slotValue).Scan(&itemGUID)
		if err != nil || itemGUID == 0 {
			return nil, nil
		}
		s.durabilityPointsLoss(ctx, uint64(itemGUID), 1)
		return nil, nil
	}
	methods["DurabilityRepair"] = func(ctx context.Context, args []any) ([]any, error) {
		pos, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if pos > math.MaxUint16 {
			return nil, fmt.Errorf("argument 1 must be uint16")
		}
		cost := true
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				cost = b
			}
		}
		discountMod, err := luaGenericFloat32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		guildBank := false
		if len(args) > 3 {
			if b, ok := args[3].(bool); ok {
				guildBank = b
			}
		}
		price, repaired := s.durabilityRepairByPos(ctx, uint16(pos), cost, float64(discountMod), guildBank)
		if repaired {
			_ = s.sendInventoryItems(ctx)
			s.sendPlayerUpdate()
		}
		return []any{price}, nil
	}
	methods["DurabilityRepairAll"] = func(ctx context.Context, args []any) ([]any, error) {
		cost := true
		if len(args) > 0 {
			if b, ok := args[0].(bool); ok {
				cost = b
			}
		}
		discountMod := float64(1)
		if len(args) > 1 {
			if value, err := luaGenericFloat32Arg(args, 1); err == nil {
				discountMod = float64(value)
			}
		}
		guildBank := false
		if len(args) > 2 {
			if b, ok := args[2].(bool); ok {
				guildBank = b
			}
		}
		return []any{s.durabilityRepairAll(ctx, cost, discountMod, guildBank)}, nil
	}
	// Eluna nearest/in-range family (WorldObjectMethods.h, see
	// lua_worldobject_range.go): replaces the old range-only
	// GetNearestGameObject bridge (2D metric, no entry/hostile args,
	// zero Lua callers) with the Eluna (range, entry, hostile) form
	// plus GetNearestPlayer/GetNearestCreature/GetPlayersInRange/
	// GetCreaturesInRange/GetGameObjectsInRange/GetNearObject/
	// GetNearObjects, registered from the player surface.
	for name, fn := range luaRangeMethods(s.server, s.luaRangeOriginForPlayer) {
		methods[name] = fn
	}
	// GetFriendlyUnitsInRange/GetUnfriendlyUnitsInRange are the Unit
	// in-range pair (UnitMethods.h:1255/1296); see
	// luaUnitFriendlyMethods in lua_worldobject_range.go.
	for name, fn := range luaUnitFriendlyMethods(s.server, s.luaRangeOriginForPlayer) {
		methods[name] = fn
	}
	// FindNearestCreature(entry, dist) is the player-surface twin of the
	// creature bridge (lua_creature_events.go:2435): the nearest live
	// creature of entry within dist yards (3D gate with both combat
	// reaches added, winner by reach-adjusted distance) on the player's
	// map+instance; nil when none. Backs the area-trigger dedup arms
	// (at_scent_larkorwi, at_sentry_point, at_brewfest).
	methods["FindNearestCreature"] = func(_ context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		dist32, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if s.server == nil || dist32 < 0 {
			return []any{nil}, nil
		}
		srcReach := float64(state.CombatReach)
		if srcReach <= 0 {
			srcReach = 1.5
		}
		dist := float64(dist32)
		s.server.motionMu.Lock()
		var best *creatureMotion
		bestAdj := dist
		for _, m := range s.server.motionMapLocked(state.Map, state.InstanceID) {
			if m.Entry != entry || m.Health == 0 {
				continue
			}
			dx := float64(m.X - state.X)
			dy := float64(m.Y - state.Y)
			dz := float64(m.Z - state.Z)
			candReach := float64(m.combatReachOrDefault())
			if d := math.Sqrt(dx*dx + dy*dy + dz*dz); d <= dist+srcReach+candReach {
				adj := d - srcReach - candReach
				if adj < 0 {
					adj = 0
				}
				if best == nil || adj < bestAdj {
					best, bestAdj = m, adj
				}
			}
		}
		s.server.motionMu.Unlock()
		if best == nil {
			return []any{nil}, nil
		}
		return []any{s.server.luaMotionCreature(best)}, nil
	}
	// SummonCreature is the player-surface twin of the creature bridge
	// (WorldObject::SummonCreature, Object.cpp:2059-2062 fallbacks via
	// the shared summonTempCreature funnel): same (entry, x, y, z, o,
	// summonType, despawnTimer-ms) signature, same 0-7 type scheme,
	// unknown entries return nil. No event-19 fire: players carry no
	// creature AI to receive JustSummoned.
	methods["SummonCreature"] = func(ctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		x, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		y, err := luaFloat32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		z, err := luaFloat32Arg(args, 3)
		if err != nil {
			return nil, err
		}
		o, err := luaFloat32Arg(args, 4)
		if err != nil {
			return nil, err
		}
		summonType := uint32(7)
		if len(args) > 5 {
			if summonType, err = luaUint32Arg(args, 5); err != nil {
				return nil, err
			}
			if summonType > 7 {
				return nil, fmt.Errorf("SummonCreature summonType must be 0-7")
			}
		}
		var despawnMs uint32
		if len(args) > 6 {
			if despawnMs, err = luaUint32Arg(args, 6); err != nil {
				return nil, err
			}
		}
		if s.server == nil {
			return []any{nil}, nil
		}
		origin := tempSummonOrigin{Map: state.Map, InstanceID: state.InstanceID, GUID: state.GUID, X: state.X, Y: state.Y, Z: state.Z, O: state.Orientation, Reach: float64(state.CombatReach)}
		summoned := s.server.summonTempCreature(ctx, origin, entry, x, y, z, o, uint8(summonType), despawnMs)
		if summoned == nil {
			return []any{nil}, nil
		}
		return []any{s.server.luaMotionCreature(summoned)}, nil
	}
	// SpawnCreature is WorldObject::SpawnCreature (WorldObjectMethods.h),
	// the Eluna 1-8 spawn-type twin of SummonCreature; the shared
	// spawnCreatureFromOrigin funnel (lua_creature_events.go) documents
	// the type translation and the seconds-to-milliseconds despawn
	// conversion. The player summoner carries no creature AI, so no
	// event-19 fire runs, matching the player SummonCreature twin.
	methods["SpawnCreature"] = func(ctx context.Context, args []any) ([]any, error) {
		if s.server == nil {
			return []any{nil}, nil
		}
		origin := tempSummonOrigin{Map: state.Map, InstanceID: state.InstanceID, GUID: state.GUID, X: state.X, Y: state.Y, Z: state.Z, O: state.Orientation, Reach: float64(state.CombatReach)}
		summoned, err := s.server.spawnCreatureFromOrigin(ctx, origin, args)
		if err != nil {
			return nil, err
		}
		if summoned == nil {
			return []any{nil}, nil
		}
		return []any{s.server.luaMotionCreature(summoned)}, nil
	}

	// luaSendMenuObjectGUID resolves the WorldObject argument shared by
	// the window-send methods below, the CHECKOBJ<WorldObject> twin:
	// only an object carrying a GUID opens a window, anything else
	// errors like the sibling player-argument methods.
	luaSendMenuObjectGUID := func(args []any) (uint64, error) {
		if len(args) == 0 {
			return 0, fmt.Errorf("argument 1 is required")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return 0, fmt.Errorf("argument 1 must be a world object")
		}
		guid, _ := objectUint64Field(obj, "GUID")
		if guid == 0 {
			return 0, fmt.Errorf("argument 1 must be a world object")
		}
		return guid, nil
	}
	// SendShowBank is WorldSession::SendShowBank (BankHandler.cpp:183-188):
	// latch the banker GUID (m_currentBankerGUID) and send SMSG_SHOW_BANK
	// carrying only that GUID. Unlike the banker-activate opcode path it
	// carries no interactability gate.
	methods["SendShowBank"] = func(ctx context.Context, args []any) ([]any, error) {
		guid, err := luaSendMenuObjectGUID(args)
		if err != nil {
			return nil, err
		}
		s.currentBankerGUID = guid
		bank := protocol.NewBuffer(8)
		bank.WriteU64(guid)
		_ = s.write(uint16(protocol.OpcodeSMSG_SHOW_BANK), bank.Bytes(), true)
		return []any{}, nil
	}
	// SendTabardVendorActivate is WorldSession::SendTabardVendorActivate
	// (NPCHandler.cpp:77-82): MSG_TABARDVENDOR_ACTIVATE carrying the
	// object's GUID, no gate.
	methods["SendTabardVendorActivate"] = func(ctx context.Context, args []any) ([]any, error) {
		guid, err := luaSendMenuObjectGUID(args)
		if err != nil {
			return nil, err
		}
		tabard := protocol.NewBuffer(8)
		tabard.WriteU64(guid)
		_ = s.write(uint16(protocol.OpcodeMSG_TABARDVENDOR_ACTIVATE), tabard.Bytes(), true)
		return []any{}, nil
	}
	// SendListInventory is WorldSession::SendListInventory
	// (ItemHandler.cpp:611-618): the vendor-list arm, whose interact
	// failure answers SMSG_SELL_ITEM with SELL_ERR_CANT_FIND_VENDOR.
	methods["SendListInventory"] = func(ctx context.Context, args []any) ([]any, error) {
		guid, err := luaSendMenuObjectGUID(args)
		if err != nil {
			return nil, err
		}
		s.sendVendorList(ctx, guid)
		return []any{}, nil
	}
	// SendTrainerList is WorldSession::SendTrainerList
	// (NPCHandler.cpp:103-115): the trainer-list arm, whose missing
	// trainer row silently sends nothing.
	methods["SendTrainerList"] = func(ctx context.Context, args []any) ([]any, error) {
		guid, err := luaSendMenuObjectGUID(args)
		if err != nil {
			return nil, err
		}
		s.sendTrainerList(ctx, guid)
		return []any{}, nil
	}
	// SendTaxiMenu is WorldSession::SendTaxiMenu (TaxiHandler.cpp:103-131):
	// the flightmaster menu arm, gated on the flightmaster flag and a
	// resolvable current node.
	methods["SendTaxiMenu"] = func(ctx context.Context, args []any) ([]any, error) {
		guid, err := luaSendMenuObjectGUID(args)
		if err != nil {
			return nil, err
		}
		s.sendTaxiMenu(ctx, guid)
		return []any{}, nil
	}
	// SendAuctionMenu is WorldSession::SendAuctionHello
	// (AuctionHouseHandler.cpp:59-75) via the shared arm: the level
	// requirement gate fires first, and the house id is the neutral id
	// (the faction->house-entry lookup has no Go model).
	methods["SendAuctionMenu"] = func(ctx context.Context, args []any) ([]any, error) {
		guid, err := luaSendMenuObjectGUID(args)
		if err != nil {
			return nil, err
		}
		s.sendAuctionHelloPacket(guid)
		return []any{}, nil
	}
	// SendShowMailBox is WorldSession::SendShowMailBox
	// (NPCHandler.cpp:84-89): SMSG_SHOW_MAILBOX carrying only the
	// postmaster GUID, defaulting to the player's own GUID like the
	// Eluna CHECKVAL default.
	methods["SendShowMailBox"] = func(ctx context.Context, args []any) ([]any, error) {
		guid := s.playerGUID
		if len(args) > 0 {
			value, err := luaUint64(args[0])
			if err != nil {
				return nil, fmt.Errorf("argument 1 must be uint64")
			}
			guid = value
		}
		mailbox := protocol.NewBuffer(8)
		mailbox.WriteU64(guid)
		_ = s.write(uint16(protocol.OpcodeSMSG_SHOW_MAILBOX), mailbox.Bytes(), true)
		return []any{}, nil
	}
	// SendCinematicStart is Player::SendCinematicStart
	// (Player.cpp:6519-6526): SMSG_TRIGGER_CINEMATIC carrying the
	// sequence id. DELTA: the C++ arm also activates the sequence's
	// first camera on the cinematic manager, which has no Go model.
	methods["SendCinematicStart"] = func(ctx context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		packet := protocol.NewBuffer(4)
		packet.WriteU32(id)
		_ = s.write(uint16(protocol.OpcodeSMSG_TRIGGER_CINEMATIC), packet.Bytes(), true)
		return []any{}, nil
	}
	// SendMovieStart is Player::SendMovieStart (Player.cpp:6529-6535):
	// SetMovie plus SMSG_TRIGGER_MOVIE carrying the movie id.
	methods["SendMovieStart"] = func(ctx context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s.player != nil {
			s.player.Movie = id
		}
		packet := protocol.NewBuffer(4)
		packet.WriteU32(id)
		_ = s.write(uint16(protocol.OpcodeSMSG_TRIGGER_MOVIE), packet.Bytes(), true)
		return []any{}, nil
	}
	// SendSpiritResurrect is WorldSession::SendSpiritResurrect
	// (NPCHandler.cpp:219-241): resurrect at half health with sickness,
	// lose a quarter durability, convert the corpse to bones, then
	// teleport to the corpse's nearest graveyard when it differs from
	// the ghost's nearest graveyard. The resurrect arm's toBones leg
	// covers the bones conversion; the explicit call after durability
	// keeps the C++ order and is a no-op by then.
	methods["SendSpiritResurrect"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.resurrectPlayerWithBones(ctx, 0.5, true, true)
		s.durabilityLossAll(ctx, 0.25, true)
		s.spawnCorpseBones(ctx)
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && s.player != nil {
			var corpseMap int64
			var corpseX, corpseY, corpseZ float64
			if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT mapId, posX, posY, posZ FROM corpse WHERE guid = ? AND corpseType <> ?", s.playerGUID, corpseTypeBones).Scan(&corpseMap, &corpseX, &corpseY, &corpseZ); err == nil {
				team := teamForRace(s.player.Race)
				corpseGrave, corpseOK := s.server.closestGraveyard(ctx, float32(corpseX), float32(corpseY), float32(corpseZ), uint32(corpseMap), s.player.Zone, team)
				ghostGrave, ghostOK := s.server.closestGraveyard(ctx, s.player.X, s.player.Y, s.player.Z, s.player.Map, s.player.Zone, team)
				if corpseOK && ghostOK && corpseGrave.ID != ghostGrave.ID {
					s.teleportTo(corpseGrave.MapID, corpseGrave.X, corpseGrave.Y, corpseGrave.Z, s.player.Orientation)
				}
			}
		}
		return []any{}, nil
	}
	// SendAddonMessage is PlayerMethods.h:3626-3650: an SMSG_MESSAGECHAT
	// with the caller's channel, LANG_ADDON (-1), the sender and
	// receiver GUIDs, and the prefix-tab-message body, delivered to the
	// receiver's session only.
	methods["SendAddonMessage"] = func(ctx context.Context, args []any) ([]any, error) {
		prefix, err := luaStringArg(args, 0)
		if err != nil {
			return nil, err
		}
		message, err := luaStringArg(args, 1)
		if err != nil {
			return nil, err
		}
		channel, err := luaUint32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		if channel > 255 {
			return nil, fmt.Errorf("argument 3 must be uint8")
		}
		receiver, err := luaGroupInviteTarget(args[3:])
		if err != nil {
			return nil, err
		}
		fullmsg := prefix + "\t" + message
		packet := protocol.NewBuffer(len(fullmsg) + 32)
		packet.WriteU8(uint8(channel))
		packet.WriteI32(-1)
		packet.WriteU64(s.playerGUID)
		packet.WriteU32(0)
		packet.WriteU64(receiver.playerGUID)
		packet.WriteU32(uint32(len(fullmsg)) + 1)
		packet.WriteCString(fullmsg)
		packet.WriteU8(0)
		_ = receiver.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet.Bytes(), true)
		return []any{}, nil
	}
	// SendGuildInvite is Guild::HandleInviteMember (Guild.cpp:1464-1512)
	// via the shared opcode handler: the invitee's name drives the
	// same guild, ignore, faction, already-guilded, already-invited
	// and rank-right gates, then the SMSG_GUILD_INVITE packet.
	methods["SendGuildInvite"] = func(ctx context.Context, args []any) ([]any, error) {
		invited, err := luaGroupInviteTarget(args)
		if err != nil {
			return nil, err
		}
		if invited.player == nil {
			return nil, fmt.Errorf("player is required")
		}
		payload := protocol.NewBuffer(len(invited.player.Name) + 1)
		payload.WriteCString(invited.player.Name)
		s.handleGuildInvite(ctx, payload.Bytes())
		return []any{}, nil
	}
	// SendQuestTemplate is PlayerMethods.h:3908-3921: the quest-details
	// window for a quest template, auto-launched by default. A missing
	// template silently sends nothing, like the C++ early return.
	// DELTA: the Go details builder bakes AutoLaunched=1, so
	// activateAccept=false still sends the auto-launched shape.
	methods["SendQuestTemplate"] = func(ctx context.Context, args []any) ([]any, error) {
		questID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if len(args) > 1 {
			if _, ok := args[1].(bool); !ok {
				return nil, fmt.Errorf("argument 2 must be boolean")
			}
		}
		data, err := s.loadQuestDetailData(ctx, questID)
		if err != nil {
			return []any{}, nil
		}
		s.sendQuestGiverQuestDetails(ctx, s.playerGUID, data)
		return []any{}, nil
	}
	// GossipSendPOI is PlayerMethods.h:3856-3874: SMSG_GOSSIP_POI with
	// flags, x, y, icon, data and the icon text, in the C++ field order.
	methods["GossipSendPOI"] = func(ctx context.Context, args []any) ([]any, error) {
		x, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		y, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		icon, err := luaUint32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		flags, err := luaUint32Arg(args, 3)
		if err != nil {
			return nil, err
		}
		data, err := luaUint32Arg(args, 4)
		if err != nil {
			return nil, err
		}
		iconText, err := luaStringArg(args, 5)
		if err != nil {
			return nil, err
		}
		packet := protocol.NewBuffer(len(iconText) + 21)
		packet.WriteU32(flags)
		packet.WriteF32(x)
		packet.WriteF32(y)
		packet.WriteU32(icon)
		packet.WriteU32(data)
		packet.WriteCString(iconText)
		_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_POI), packet.Bytes(), true)
		return []any{}, nil
	}
	// SendPacket is Player::SendPacket (PlayerMethods.h:3599-3615): the
	// CreatePacket-built WorldPacket goes to the player's session when
	// selfOnly (default true); otherwise it fans out via SendMessageToSet
	// (broadcastToNearby excludes the source, so the self write plus the
	// fan-out matches the C++ self=true leg). A non-packet arg errors
	// like Eluna's CHECKOBJ; an opcode above uint16 errors instead of
	// silently truncating to another opcode.
	methods["SendPacket"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("packet is required")
		}
		pkt, ok := args[0].(*scripting.Packet)
		if !ok || pkt == nil {
			return nil, fmt.Errorf("packet must be a WorldPacket")
		}
		if pkt.Opcode > math.MaxUint16 {
			return nil, fmt.Errorf("packet opcode must fit uint16")
		}
		selfOnly := true
		if len(args) > 1 && args[1] != nil {
			value, ok := args[1].(bool)
			if !ok {
				return nil, fmt.Errorf("argument 2 must be boolean")
			}
			selfOnly = value
		}
		opcode := uint16(pkt.Opcode)
		if !selfOnly && s.server != nil {
			s.server.broadcastToNearby(opcode, pkt.Data, s)
		}
		_ = s.write(opcode, pkt.Data, true)
		return []any{}, nil
	}
	// GossipAddQuests is Player::GossipAddQuests (PlayerMethods.h:3885-3900):
	// a questgiver-flagged creature or QUESTGIVER-type gameobject stages
	// its quest menu into the pending gossip menu (PrepareQuestMenu,
	// Player.cpp:14748-14803), which clears the previous quest list
	// first and rebuilds it from the starter/ender relations via the
	// shared loadCreatureQuestMenu / loadGameObjectQuestMenu arms. A
	// non-questgiver source, or one that does not resolve, stages
	// nothing and leaves the pending menu untouched, matching the C++
	// early return before ClearMenu. The creature flag reads the
	// object's NPCFlags field, falling back to creature_template.npcflag
	// for event-surface objects, which carry no such field (the
	// GetGossipTextId fallback pattern). Missing player or stores
	// silently stage nothing, like RewardQuest's nil guard.
	methods["GossipAddQuests"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("worldobject is required")
		}
		obj, ok := args[0].(*scripting.Object)
		if !ok || obj == nil {
			return nil, fmt.Errorf("worldobject is required")
		}
		entry, ok := objectUint32Field(obj, "Entry")
		if !ok {
			return nil, fmt.Errorf("worldobject is required")
		}
		if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return []any{}, nil
		}
		var quests []gossipQuestItem
		var err error
		switch obj.Type {
		case "Creature":
			flags, hasFlags := objectUint32Field(obj, "NPCFlags")
			if !hasFlags {
				var npcflag int64
				if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT npcflag FROM creature_template WHERE entry = ?", entry).Scan(&npcflag); err == nil {
					flags = uint32(npcflag)
				}
			}
			if flags&npcFlagQuestgiver == 0 {
				return []any{}, nil
			}
			quests, err = s.loadCreatureQuestMenu(ctx, entry, s.player.Level)
		case "GameObject":
			var goType int64
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT type FROM gameobject_template WHERE entry = ?", entry).Scan(&goType)
			if goType != 2 {
				return []any{}, nil
			}
			quests, err = s.loadGameObjectQuestMenu(ctx, entry, s.player.Level)
		default:
			return []any{}, nil
		}
		if err != nil {
			return nil, err
		}
		if s.gossip == nil {
			s.gossip = &gossipMenuState{Items: make(map[uint32]gossipMenuItem)}
		}
		s.gossip.Quests = quests
		return []any{}, nil
	}
	// UnbindInstance is Player::UnbindInstance (PlayerMethods.h:2427-2440;
	// Player.cpp:19099-19115): the named map's bind at the given
	// difficulty (default 0, silent no-op at or above MAX_DIFFICULTY 4,
	// DBCEnums.h:296) is dropped via the shared unbind arm. Unlike
	// UnbindAllInstances it does NOT skip the player's current map:
	// the Eluna arm unbinds exactly the named map.
	methods["UnbindInstance"] = func(ctx context.Context, args []any) ([]any, error) {
		mapID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		difficulty := uint32(0)
		if len(args) > 1 && args[1] != nil {
			difficulty, err = luaUint32Arg(args, 1)
			if err != nil {
				return nil, err
			}
		}
		if difficulty >= 4 {
			return []any{}, nil
		}
		s.unbindInstanceBinds(ctx, func(b instanceBindRow) bool {
			return b.mapID == mapID && uint32(b.difficulty) == difficulty
		})
		return []any{}, nil
	}
	// UnbindAllInstances is Player::UnbindAllInstances
	// (PlayerMethods.h:2446-2485): every bind except the one for the
	// map the player currently is in is dropped via the shared unbind
	// arm, difficulties 0..3 like the C++ MAX_DIFFICULTY loop.
	methods["UnbindAllInstances"] = func(ctx context.Context, args []any) ([]any, error) {
		if s == nil || s.player == nil {
			return []any{}, nil
		}
		current := s.player.Map
		s.unbindInstanceBinds(ctx, func(b instanceBindRow) bool {
			return b.mapID != current
		})
		return []any{}, nil
	}
	// GiveXP is Player::GiveXP (Player.cpp:2642-2656) via the shared
	// grant arm: the dead, NO_XP_GAIN and loot-recipient gates, rested
	// and recruit-a-friend bonuses, SMSG_LOG_XPGAIN and level-ups. The
	// victim is optional; a nil victim grants non-kill XP.
	methods["GiveXP"] = func(ctx context.Context, args []any) ([]any, error) {
		xp, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		var victimGUID uint64
		if len(args) > 1 {
			if obj, ok := args[1].(*scripting.Object); ok && obj != nil {
				victimGUID, _ = objectUint64Field(obj, "GUID")
			}
		}
		s.grantXPWithVictim(ctx, xp, victimGUID)
		return []any{}, nil
	}
	// LeaveBattleground is Player::LeaveBattleground
	// (PlayerMethods.h:2493-2501): the voluntary-leave funnel, which
	// teleports to the entry point unless teleToEntryPoint is false.
	methods["LeaveBattleground"] = func(ctx context.Context, args []any) ([]any, error) {
		tele := true
		if len(args) > 0 {
			value, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("argument 1 must be boolean")
			}
			tele = value
		}
		s.handleLeaveBattlefield(ctx, nil, false, !tele)
		return []any{}, nil
	}
	// RemoveFromBattlegroundRaid is
	// Player::RemoveFromBattlegroundOrBattlefieldRaid
	// (Player.cpp:24486-24505): unlink the BG raid and re-link the
	// stashed original group when one exists.
	methods["RemoveFromBattlegroundRaid"] = func(ctx context.Context, _ []any) ([]any, error) {
		if s.server != nil {
			s.server.removePlayerFromBGRaid(s)
		}
		return []any{}, nil
	}

	return &scripting.Object{Type: "Player", Fields: fields, Methods: methods}
}

func (s *session) triggerMapEntryEvent(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	mapObject := scripting.NewMapObject(s.player.Map, s.player.InstanceID, s.server.mapLive(s.player.Map, s.player.InstanceID))
	if _, err := s.server.Features.Scripts.TriggerMapEvent(ctx, s.player.Map, scripting.MapEventOnPlayerEnter, mapObject, s.luaPlayer()); err != nil {
		s.debug("lua map entry event failed", "map", s.player.Map, "instance", s.player.InstanceID, "error", err)
	}
}

// triggerMapLeaveEvent dispatches Eluna MAP_EVENT_ON_PLAYER_LEAVE (22) as
// (event, map, player), fired from Map::RemovePlayerFromMap (Maps/Map.cpp:1011)
// via ScriptMgr::OnPlayerLeaveMap. The map argument is the map being left, so
// far-teleport callers pass the origin pair and logout callers pass the live
// pair; the entry twin takes the destination from the live player instead.
func (s *session) triggerMapLeaveEvent(ctx context.Context, mapID, instanceID uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	mapObject := scripting.NewMapObject(mapID, instanceID, s.server.mapLive(mapID, instanceID))
	if _, err := s.server.Features.Scripts.TriggerMapEvent(ctx, mapID, scripting.MapEventOnPlayerLeave, mapObject, s.luaPlayer()); err != nil {
		s.debug("lua map leave event failed", "map", mapID, "instance", instanceID, "error", err)
	}
}

func (s *session) luaMessageMethod() scripting.ObjectMethod {
	return func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("message is required")
		}
		message, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("message must be a string")
		}
		return nil, s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), protocol.BuildSystemChatMessage(message), true)
	}
}

// unitClassRaceMask is Unit::GetClassMask/GetRaceMask (Unit.h:849/852):
// 1 << (value - 1). A zero value reads 0 here: the creature race byte
// is never set by Creature::InitEntry (which sets only the class, from
// template unit_class, Creature.cpp:494), and the C++ shift of a wrapped
// uint8 is undefined, so Go saturates instead of replicating it. Values
// above 32 also read 0.
func unitClassRaceMask(value uint32) uint32 {
	if value == 0 || value > 32 {
		return 0
	}
	return 1 << (value - 1)
}

// dbcLocaleArg is the optional locale argument of the Eluna name
// getters (UnitMethods.h:1152/1195): CHECKVAL with DEFAULT_LOCALE, so a
// missing argument reads enUS (0).
func dbcLocaleArg(args []any) (uint32, error) {
	if len(args) == 0 {
		return 0, nil
	}
	return luaUint32Arg(args, 0)
}

// dbcLocaleName is Unit::GetClassAsString/GetRaceAsString
// (UnitMethods.h:1152/1195): the ChrClasses/ChrRaces Name[locale] string.
// ChrClasses Name columns are DBC fields 4-19 and ChrRaces Name columns
// are fields 14-29 (DBCStructure.h:388/431); locales above ruRU (8)
// error like C++'s luaL_argerror, matching the LocaleConstant enum
// (Common.h: TOTAL_LOCALES = 9). A missing row, file, or store reads
// nil, like C++ pushing nothing for a failed LookupEntry.
func (srv *Server) dbcLocaleName(file string, id uint32, nameField int, locale uint32) (any, error) {
	if locale >= 9 {
		return nil, fmt.Errorf("argument 1 must be a valid LocaleConstant")
	}
	if srv == nil || srv.Data == nil {
		return nil, nil
	}
	dbcFile, err := srv.Data.File(file)
	if err != nil || dbcFile == nil {
		return nil, nil
	}
	record, ok := dbcFile.Find(id)
	if !ok {
		return nil, nil
	}
	name, err := record.String(nameField + int(locale))
	if err != nil {
		return nil, nil
	}
	return name, nil
}

// unitControllerGUID is Unit::GetCharmerOrOwnerGUID (Unit.h:1170):
// the charmer GUID when charmed, else the owner GUID.
func unitControllerGUID(charmed bool, charmerGUID, ownerGUID uint64) uint64 {
	if charmed {
		return charmerGUID
	}
	return ownerGUID
}

func luaNoArgs(value func() any) scripting.ObjectMethod {
	return func(_ context.Context, _ []any) ([]any, error) { return []any{value()}, nil }
}

func playerPowerIndex(args []any, class uint8) (int, error) {
	power := uint32(classPowerType(class))
	if len(args) > 0 {
		value, err := luaUint32Arg(args, 0)
		if err != nil {
			return 0, err
		}
		power = value
	}
	if power >= 7 {
		return 0, fmt.Errorf("power type %d is invalid", power)
	}
	return int(power), nil
}

// destroyItemEntryCount is the shared Player::DestroyItemCount(entry, ...)
// walk behind the DestroyItemCount bridge and the RemoveItem entry arm:
// it walks the player's stacks of the entry in bag/slot order, deleting
// whole stacks and trimming the stack that covers the remainder, and
// returns the total actually destroyed. C++'s partial-stack branch
// returns only the full-stack subtotal; this bridge returns the full
// destroyed total instead (the rows left behind are the same). The
// update flag mirrors C++'s send-update leg: a live inventory refresh
// is pushed only when it is true. The C++ in-trade skip has no Go row
// model and is not replicated.
func (s *session) destroyItemEntryCount(ctx context.Context, item, count uint32, update bool) (uint32, error) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || count == 0 {
		return 0, nil
	}
	db := s.server.CharactersStore.DB
	rows, err := db.QueryContext(ctx, "SELECT ci.item, ii.count FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ? ORDER BY ci.bag, ci.slot", s.playerGUID, item)
	if err != nil {
		if isMissingTableError(err) {
			return 0, nil
		}
		return 0, err
	}
	type stack struct {
		guid  int64
		count int64
	}
	var stacks []stack
	for rows.Next() {
		var st stack
		if err := rows.Scan(&st.guid, &st.count); err == nil {
			stacks = append(stacks, st)
		}
	}
	rows.Close()
	remaining := int64(count)
	destroyed := uint32(0)
	for _, st := range stacks {
		if remaining <= 0 {
			break
		}
		if st.count <= remaining {
			// This bridge is the Player::DestroyItemCount twin, so
			// each fully-consumed stack reaches DestroyItem and fires
			// ITEM_EVENT_ON_REMOVE (5); the trimming stack only
			// decrements and never fires.
			s.fireItemRemoveHook(ctx, uint64(st.guid))
			if _, err := db.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, st.guid); err != nil {
				return 0, err
			}
			if _, err := db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", st.guid); err != nil {
				return 0, err
			}
			s.adjustQuestItemCount(ctx, item, uint32(st.count), false)
			s.applyItemObtainSpells(ctx, item, 0, false)
			s.despawnItem(uint64(st.guid))
			destroyed += uint32(st.count)
			remaining -= st.count
		} else {
			if _, err := db.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", remaining, st.guid); err != nil {
				return 0, err
			}
			s.adjustQuestItemCount(ctx, item, uint32(remaining), false)
			destroyed += uint32(remaining)
			remaining = 0
		}
	}
	if update && destroyed > 0 {
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
	}
	return destroyed, nil
}

// luaItemGUIDArg resolves an Eluna Item object argument to its instance
// GUID, the CHECKOBJ<Item> twin: a non-item argument errors, while an
// item object without a GUID reads 0 so callers can take the C++ nil-item
// no-op arm.
func luaItemGUIDArg(args []any, index int) (uint64, error) {
	if index >= len(args) {
		return 0, fmt.Errorf("argument %d is required", index+1)
	}
	obj, ok := args[index].(*scripting.Object)
	if !ok || obj == nil {
		return 0, fmt.Errorf("argument %d must be an item", index+1)
	}
	guid, ok := objectUint64Field(obj, "GUID")
	if !ok {
		return 0, fmt.Errorf("argument %d must be an item", index+1)
	}
	return guid, nil
}

func luaUint32Arg(args []any, index int) (uint32, error) {
	if index >= len(args) {
		return 0, fmt.Errorf("argument %d is required", index+1)
	}
	value, err := luaUint64(args[index])
	if err != nil || value > math.MaxUint32 {
		return 0, fmt.Errorf("argument %d must be uint32", index+1)
	}
	return uint32(value), nil
}

// luaQuestStatusRowData is one recorded quest status row for the Eluna
// quest-helper methods: the DB character_queststatus row when readable,
// else the in-memory quest log slot (whose State maps 1 to COMPLETE,
// 2 to FAILED, anything else to INCOMPLETE).
type luaQuestStatusRowData struct {
	found       bool
	status      int64
	counters    [questObjectivesCount]uint16
	itemCounts  [questItemObjectivesCount]uint16
	playerCount uint16
	explored    bool
	timer       uint32
}

func (s *session) luaQuestStatusRow(ctx context.Context, questID uint32) luaQuestStatusRowData {
	if s != nil && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var status, explored, timer, mob1, mob2, mob3, mob4, item1, item2, item3, item4, item5, item6, playerCount int64
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT status, explored, timer, mobcount1, mobcount2, mobcount3, mobcount4, itemcount1, itemcount2, itemcount3, itemcount4, itemcount5, itemcount6, playercount FROM character_queststatus WHERE guid = ? AND quest = ?`, s.playerGUID, questID).Scan(&status, &explored, &timer, &mob1, &mob2, &mob3, &mob4, &item1, &item2, &item3, &item4, &item5, &item6, &playerCount)
		if err == nil {
			return luaQuestStatusRowData{found: true, status: status, counters: [questObjectivesCount]uint16{uint16(mob1), uint16(mob2), uint16(mob3), uint16(mob4)}, itemCounts: [questItemObjectivesCount]uint16{uint16(item1), uint16(item2), uint16(item3), uint16(item4), uint16(item5), uint16(item6)}, playerCount: uint16(playerCount), explored: explored != 0, timer: uint32(timer)}
		}
	}
	if s != nil && s.player != nil {
		for slot := 0; slot < playerQuestLogSlots; slot++ {
			entry := s.player.QuestLog[slot]
			if entry.QuestID != questID {
				continue
			}
			status := int64(questStatusIncomplete)
			if entry.State == questCompleteStateFlag(questStatusComplete) {
				status = questStatusComplete
			} else if entry.State == questCompleteStateFlag(questStatusFailed) {
				status = questStatusFailed
			}
			return luaQuestStatusRowData{found: true, status: status, counters: entry.Counters, itemCounts: entry.ItemCounts, playerCount: entry.PlayerCount, explored: entry.Explored, timer: entry.Timer}
		}
	}
	return luaQuestStatusRowData{}
}

// luaQuestLogIDs lists the quest ids in the player's quest log in slot
// order (the C++ GetQuestSlotQuestId iteration the HasQuestFor* methods
// scan); without a player state it falls back to the recorded status
// rows, which the callers filter by status anyway.
func (s *session) luaQuestLogIDs(ctx context.Context) []uint32 {
	if s != nil && s.player != nil {
		ids := make([]uint32, 0, playerQuestLogSlots)
		for slot := 0; slot < playerQuestLogSlots; slot++ {
			if questID := s.player.QuestLog[slot].QuestID; questID != 0 {
				ids = append(ids, questID)
			}
		}
		return ids
	}
	if s != nil && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT quest FROM character_queststatus WHERE guid = ? ORDER BY quest", s.playerGUID)
		if err == nil {
			defer rows.Close()
			ids := make([]uint32, 0)
			for rows.Next() {
				var questID int64
				if err := rows.Scan(&questID); err == nil && questID > 0 {
					ids = append(ids, uint32(questID))
				}
			}
			return ids
		}
	}
	return nil
}

// luaQuestObjectives loads the objective columns of a quest template;
// a load failure reads as no objectives (the callers then report no
// match), which is the safe direction for a minimal-schema template.
func (s *session) luaQuestObjectives(ctx context.Context, questID uint32) (questQueryData, bool) {
	data, err := s.loadQuestQueryData(ctx, questID)
	if err != nil {
		return questQueryData{}, false
	}
	return data, true
}

// luaQuestSourceItemDroppable is the source-item (ItemDrop) arm of
// Player::HasQuestForItem: a unique item is droppable while the
// inventory-plus-bank count is below MaxCount, otherwise the custom
// ItemDropQuantity, otherwise the stack size.
func (s *session) luaQuestSourceItemDroppable(ctx context.Context, questID uint32, index int, entry uint32) bool {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var maxCount, stackable int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MaxCount, 0), COALESCE(Stackable, 1) FROM item_template WHERE entry = ?", entry).Scan(&maxCount, &stackable); err != nil {
		return false
	}
	owned, err := s.questInventoryItemCount(ctx, s.playerGUID, entry, true)
	if err != nil {
		return false
	}
	if maxCount != 0 && int64(owned) < maxCount {
		return true
	}
	if stackable < 1 {
		stackable = 1
	}
	var quantities [questObjectivesCount]int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(ItemDropQuantity1, 0), COALESCE(ItemDropQuantity2, 0), COALESCE(ItemDropQuantity3, 0), COALESCE(ItemDropQuantity4, 0) FROM quest_template WHERE ID = ?", questID).Scan(&quantities[0], &quantities[1], &quantities[2], &quantities[3]); err == nil && index >= 0 && index < len(quantities) {
		if quantities[index] != 0 {
			return int64(owned) < quantities[index]
		}
	}
	return int64(owned) < stackable
}

// elunaCanCompleteQuest mirrors Player::CanCompleteQuest
// (Player.cpp:14970-15040) in Eluna order: a missing template reads
// false, the rewarded gate uses GetQuestRewardStatus (repeatable quests
// never report rewarded, seasonal quests report their cooldown — the
// session getQuestRewardStatus twin), the auto-complete arm fires only
// for quests with no recorded status and a passing CanTakeQuest, and
// the INCOMPLETE objective checks read the stored status row: deliver
// item counts, kill/cast/speak-to counts (the computed-flag arms are
// count-gated, so the per-slot checks subsume them), player kills, the
// exploration flag, the timed expiry, required money, and the first
// reputation objective. It deliberately differs from the accept-path
// canCompleteQuest twin, which reads live inventory counts for the
// deliver arm and blocks on any rewarded row.
func (s *session) elunaCanCompleteQuest(ctx context.Context, questID uint32) bool {
	if questID == 0 || s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	wdb := s.server.WorldStore.DB
	var flags, reqNpc1, reqNpc2, reqNpc3, reqNpc4, reqNpcCount1, reqNpcCount2, reqNpcCount3, reqNpcCount4 int64
	var reqItem1, reqItem2, reqItem3, reqItem4, reqItem5, reqItem6 int64
	var reqItemCount1, reqItemCount2, reqItemCount3, reqItemCount4, reqItemCount5, reqItemCount6 int64
	var reqMoney, repFaction, repValue, requiredPlayerKills int64
	err := wdb.QueryRowContext(ctx, `SELECT Flags,
		RequiredNpcOrGo1, RequiredNpcOrGo2, RequiredNpcOrGo3, RequiredNpcOrGo4,
		RequiredNpcOrGoCount1, RequiredNpcOrGoCount2, RequiredNpcOrGoCount3, RequiredNpcOrGoCount4,
		RequiredItemId1, RequiredItemId2, RequiredItemId3, RequiredItemId4, RequiredItemId5, RequiredItemId6,
		RequiredItemCount1, RequiredItemCount2, RequiredItemCount3, RequiredItemCount4, RequiredItemCount5, RequiredItemCount6,
		COALESCE(RewOrReqMoney, 0), COALESCE(RepObjectiveFaction, 0), COALESCE(RepObjectiveValue, 0),
		COALESCE(RequiredPlayerKills, 0)
		FROM quest_template WHERE ID = ?`, questID).Scan(&flags,
		&reqNpc1, &reqNpc2, &reqNpc3, &reqNpc4,
		&reqNpcCount1, &reqNpcCount2, &reqNpcCount3, &reqNpcCount4,
		&reqItem1, &reqItem2, &reqItem3, &reqItem4, &reqItem5, &reqItem6,
		&reqItemCount1, &reqItemCount2, &reqItemCount3, &reqItemCount4, &reqItemCount5, &reqItemCount6,
		&reqMoney, &repFaction, &repValue, &requiredPlayerKills)
	if err != nil {
		if err2 := wdb.QueryRowContext(ctx, "SELECT Flags FROM quest_template WHERE ID = ?", questID).Scan(&flags); err2 != nil {
			return false
		}
	}
	if s.getQuestRewardStatus(ctx, questID) {
		return false
	}
	row := s.luaQuestStatusRow(ctx, questID)
	inLog := row.found
	if !inLog {
		for slot := 0; slot < playerQuestLogSlots; slot++ {
			if s.player.QuestLog[slot].QuestID == questID {
				inLog = true
				break
			}
		}
	}
	if !inLog {
		if s.questIsAutoComplete(ctx, questID, uint32(flags)) {
			if canTake, takeErr := s.canTakeQuest(ctx, questID); takeErr == nil && canTake {
				return true
			}
		}
		return false
	}
	if row.status != questStatusIncomplete {
		return false
	}
	reqItemCounts := [questItemObjectivesCount]int64{reqItemCount1, reqItemCount2, reqItemCount3, reqItemCount4, reqItemCount5, reqItemCount6}
	for index := range reqItemCounts {
		if reqItemCounts[index] != 0 && int64(row.itemCounts[index]) < reqItemCounts[index] {
			return false
		}
	}
	reqNpcs := [questObjectivesCount]int64{reqNpc1, reqNpc2, reqNpc3, reqNpc4}
	reqNpcCounts := [questObjectivesCount]int64{reqNpcCount1, reqNpcCount2, reqNpcCount3, reqNpcCount4}
	for index := range reqNpcs {
		if reqNpcs[index] != 0 && reqNpcCounts[index] != 0 && int64(row.counters[index]) < reqNpcCounts[index] {
			return false
		}
	}
	if requiredPlayerKills != 0 && int64(row.playerCount) < requiredPlayerKills {
		return false
	}
	var specialFlags int64
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
	if specialFlags&questSpecialExplorationOrEvent != 0 && !row.explored {
		return false
	}
	if specialFlags&questSpecialFlagsTimed != 0 && row.timer <= uint32(time.Now().Unix()) {
		return false
	}
	if reqMoney < 0 && int64(s.player.Money) < -reqMoney {
		return false
	}
	if repFaction > 0 && int64(s.reputationStanding(uint32(repFaction))) < repValue {
		return false
	}
	return true
}

func luaStringArg(args []any, index int) (string, error) {
	if index >= len(args) {
		return "", fmt.Errorf("argument %d is required", index+1)
	}
	message, ok := args[index].(string)
	if !ok {
		return "", fmt.Errorf("argument %d must be a string", index+1)
	}
	return message, nil
}

func luaUint64(value any) (uint64, error) {
	switch value := value.(type) {
	case scripting.UInt64:
		return uint64(value), nil
	case scripting.Int64:
		if value >= 0 {
			return uint64(value), nil
		}
	case uint8:
		return uint64(value), nil
	case uint16:
		return uint64(value), nil
	case uint32:
		return uint64(value), nil
	case uint64:
		return value, nil
	case int:
		if value >= 0 {
			return uint64(value), nil
		}
	case int64:
		if value >= 0 {
			return uint64(value), nil
		}
	case float64:
		if value >= 0 && value <= math.MaxUint64 && value == math.Trunc(value) {
			return uint64(value), nil
		}
	case float32:
		if value >= 0 && value == float32(math.Trunc(float64(value))) {
			return uint64(value), nil
		}
	case string:
		return strconv.ParseUint(value, 10, 64)
	}
	return 0, fmt.Errorf("value is not unsigned")
}

func luaInt64Arg(args []any, index int) (int64, error) {
	if index >= len(args) {
		return 0, fmt.Errorf("argument %d is required", index+1)
	}
	switch value := args[index].(type) {
	case int:
		return int64(value), nil
	case int64:
		return value, nil
	case float64:
		if value >= math.MinInt64 && value <= math.MaxInt64 && value == math.Trunc(value) {
			return int64(value), nil
		}
	case string:
		return strconv.ParseInt(value, 10, 64)
	}
	return 0, fmt.Errorf("argument %d must be int64", index+1)
}

func luaFloat32Arg(args []any, index int) (float32, error) {
	if index >= len(args) {
		return 0, fmt.Errorf("argument %d is required", index+1)
	}
	var value float64
	switch current := args[index].(type) {
	case float64:
		value = current
	case float32:
		value = float64(current)
	case int:
		value = float64(current)
	case int64:
		value = float64(current)
	default:
		return 0, fmt.Errorf("argument %d must be number", index+1)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > maxPositionCoordinate {
		return 0, fmt.Errorf("argument %d is outside position limits", index+1)
	}
	return float32(value), nil
}

func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "no such table") || strings.Contains(value, "doesn't exist") || strings.Contains(value, "unknown table")
}
