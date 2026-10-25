package world

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

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
	methods["GetObjectType"] = luaNoArgs(func() any { return "Player" })
	methods["GetZ"] = luaNoArgs(func() any { return state.Z })
	methods["GetLevel"] = luaNoArgs(func() any { return state.Level })
	methods["GetRace"] = luaNoArgs(func() any { return state.Race })
	methods["GetClass"] = luaNoArgs(func() any { return state.Class })
	methods["GetGender"] = luaNoArgs(func() any { return state.Gender })
	// GetDisplayId is Unit::GetDisplayId (Unit.h): the UNIT_FIELD_DISPLAYID
	// read. Go computes the same value the update-values builder broadcasts
	// (player_state.go): race male/female display by gender, overridden by
	// the active transform display. Backs the maladaar stolen-soul and
	// kiljaeden sinister-reflection SetDisplayId arms.
	methods["GetDisplayId"] = luaNoArgs(func() any {
		displayID := uint32(0)
		if s.server != nil && s.server.Data != nil {
			if race, found, err := s.server.Data.Race(uint32(state.Race)); err == nil && found {
				displayID = race.MaleDisplayID
				if state.Gender != 0 {
					displayID = race.FemaleDisplayID
				}
			}
		}
		if state.TransformDisplayID != 0 {
			displayID = state.TransformDisplayID
		}
		return displayID
	})
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
	methods["GetTeam"] = luaNoArgs(func() any { return teamForRace(state.Race) })
	methods["GetMaxHealth"] = luaNoArgs(func() any { return state.MaxHealth })
	methods["GetHealth"] = luaNoArgs(func() any { return state.Health })
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
	methods["HasSpellCooldown"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		now := time.Now().UnixMilli()
		for _, cooldown := range state.Cooldowns {
			if cooldown.Spell == spell && cooldown.End > now {
				return []any{true}, nil
			}
		}
		return []any{false}, nil
	}
	methods["IsInWater"] = luaNoArgs(func() any { return s.isSwimming })
	methods["IsMoving"] = luaNoArgs(func() any { return s.isMoving })
	methods["CanFly"] = luaNoArgs(func() any { return state.MountDisplayID != 0 })
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
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || count == 0 {
			return []any{uint32(0)}, nil
		}
		db := s.server.CharactersStore.DB
		rows, err := db.QueryContext(ctx, "SELECT ci.item, ii.count FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ? ORDER BY ci.bag, ci.slot", state.GUID, item)
		if err != nil {
			if isMissingTableError(err) {
				return []any{uint32(0)}, nil
			}
			return nil, err
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
				if _, err := db.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", state.GUID, st.guid); err != nil {
					return nil, err
				}
				if _, err := db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", st.guid); err != nil {
					return nil, err
				}
				s.adjustQuestItemCount(ctx, item, uint32(st.count), false)
				s.applyItemObtainSpells(ctx, item, 0, false)
				s.despawnItem(uint64(st.guid))
				destroyed += uint32(st.count)
				remaining -= st.count
			} else {
				if _, err := db.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", remaining, st.guid); err != nil {
					return nil, err
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
		return []any{destroyed}, nil
	}
	methods["HasAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		_, ok := s.auras[spell]
		return []any{ok}, nil
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
		s.nearTeleportMove(position[0], position[1], position[2], position[3])
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
	// SendMessageToSet — self write plus nearby broadcast skipping self.
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
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes(), s)
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
	methods["GetNearestGameObject"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return []any{nil}, nil
		}
		rangeValue, err := luaFloat32Arg(args, 0)
		if err != nil || rangeValue < 0 {
			return []any{nil}, err
		}
		object := s.nearestGameObject(ctx, state.Map, state.X, state.Y, rangeValue)
		return []any{object}, nil
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
	return &scripting.Object{Type: "Player", Fields: fields, Methods: methods}
}

func (s *session) triggerMapEntryEvent(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	mapObject := scripting.NewMapObject(s.player.Map, s.player.InstanceID, mapLiveFromStore(s.server.Data, s.player.Map))
	if _, err := s.server.Features.Scripts.TriggerMapEvent(ctx, s.player.Map, scripting.MapEventOnPlayerEnter, mapObject, s.luaPlayer()); err != nil {
		s.debug("lua map entry event failed", "map", s.player.Map, "instance", s.player.InstanceID, "error", err)
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
