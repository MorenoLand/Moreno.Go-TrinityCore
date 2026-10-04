package world

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Reference: SharedDefines.h:3601 (enum TradeStatus)
const (
	tradeStatusBusy          uint32 = 0
	tradeStatusBeginTrade    uint32 = 1
	tradeStatusOpenWindow    uint32 = 2
	tradeStatusTradeCanceled uint32 = 3
	tradeStatusTradeAccept   uint32 = 4
	tradeStatusBusy2         uint32 = 5
	tradeStatusNoTarget      uint32 = 6
	tradeStatusBackToTrade   uint32 = 7
	tradeStatusTradeComplete uint32 = 8
	tradeStatusTradeRejected uint32 = 9
	tradeStatusTargetTooFar  uint32 = 10
	tradeStatusWrongFaction  uint32 = 11
	tradeStatusCloseWindow   uint32 = 12
	tradeStatusIgnoreYou     uint32 = 14
	tradeStatusYouStunned    uint32 = 15
	tradeStatusTargetStunned uint32 = 16
	tradeStatusYouDead       uint32 = 17
	tradeStatusTargetDead    uint32 = 18
	tradeStatusYouLogout     uint32 = 19
	tradeStatusTargetLogout  uint32 = 20
	tradeStatusTrialAccount  uint32 = 21
	tradeStatusWrongRealm    uint32 = 22
	tradeStatusNotOnTaplist  uint32 = 23

	tradeSlotCount       = 7
	tradeSlotTradedCount = 6
	tradeSlotNonTraded   = 6
	// TRADE_DISTANCE (ObjectDefines.h:27) — 11.11f. Compared in 2D
	// (IsWithinDistInMap(..., false), Object.cpp:1166).
	tradeDistance = 11.11
)

type tradeSlotItem struct {
	ItemGUID         uint64
	ItemEntry        uint32
	DisplayID        uint32
	StackCount       uint32
	EnchantID        uint32
	GiftCreatorGUID  uint64
	CreatorGUID      uint64
	Wrapped          bool
	RandomPropertyID uint32
	Durability       uint32
	MaxDurability    uint32
	LockID           uint32
}

type playerTradeState struct {
	Partner  *session
	Money    uint32
	Items    map[uint8]tradeSlotItem
	Accepted bool
	// Deferred-spell trade state (TradeData::_spell/_spellCastItem/
	// _acceptProccess, TradeData.h): an enchant cast on the trade window's
	// non-traded slot is stored here and applied when the trade executes.
	SpellID           uint32
	SpellCastItemGUID uint64
	// InAcceptProcess mirrors TradeData::_acceptProccess (TradeData.h:60-62):
	// true while both sides have accepted and the trade is executing.
	// InTradeItems mirrors Item::mb_in_trade (Item.h:107-108, 215) for the
	// traded items snapshotted at setAcceptTradeMode: while set, inventory-
	// counting paths must skip these items (Player::HasItemCount,
	// Player::DestroyItemCount, Player::CanStoreItems skip IsInTrade items).
	InAcceptProcess bool
	InTradeItems    map[uint64]bool
}

// setTradeSpell mirrors TradeData::SetSpell (TradeData.cpp:80-94): no-op when
// the stored spell is unchanged; otherwise stores the spell, un-accepts both
// sides, and pushes the extended trade update to both clients (Update(true)
// + Update(false)).
func (s *session) setTradeSpell(spellID uint32, castItemGUID uint64) {
	if s.trade == nil {
		return
	}
	if s.trade.SpellID == spellID && s.trade.SpellCastItemGUID == castItemGUID {
		return
	}
	s.trade.SpellID = spellID
	s.trade.SpellCastItemGUID = castItemGUID
	if s.trade.Accepted {
		s.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
	}
	if s.trade.Partner != nil && s.trade.Partner.trade != nil {
		if s.trade.Partner.trade.Accepted {
			s.trade.Partner.trade.Accepted = false
			_ = s.trade.Partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
		}
		s.notifyTradeUpdate()
	}
}

// setAcceptTradeMode mirrors the static setAcceptTradeMode
// (TradeHandler.cpp:209-233): both trades enter the accept process
// (TradeData::SetInAcceptProcess(true)) and the six traded items of each
// side are snapshotted and flagged in-trade (Item::SetInTrade).
func setAcceptTradeMode(my, partner *session) {
	for _, st := range []*session{my, partner} {
		if st == nil || st.trade == nil {
			continue
		}
		st.trade.InAcceptProcess = true
		st.trade.InTradeItems = make(map[uint64]bool)
		for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
			if it, ok := st.trade.Items[slot]; ok && it.ItemGUID != 0 {
				st.trade.InTradeItems[it.ItemGUID] = true
			}
		}
	}
}

// clearAcceptTradeMode mirrors the two clearAcceptTradeMode overloads
// (TradeHandler.cpp:234-248): both trades leave the accept process
// (TradeData::SetInAcceptProcess(false)) and the in-trade item flags are
// cleared (Item::SetInTrade(false)).
func clearAcceptTradeMode(my, partner *session) {
	for _, st := range []*session{my, partner} {
		if st == nil || st.trade == nil {
			continue
		}
		st.trade.InAcceptProcess = false
		st.trade.InTradeItems = nil
	}
}

// tradeSpellStillCastable mirrors the deferred-spell guard block in
// HandleAcceptTradeOpcode (TradeHandler.cpp:364-392): the stored spell ID
// must resolve to a spell entry (sSpellMgr->GetSpellInfo), the trade
// target's non-traded slot item must still be present (his_trade->GetItem
// (TRADE_SLOT_NONTRADED) for the acceptor's spell; my_trade's for the
// partner's — caller passes the target session accordingly), and a stored
// cast-item GUID must still resolve in the caster's inventory (TradeData::
// GetSpellCastItem = GetItemByGuid(_spellCastItem)). The CheckCast(true)
// re-validation that follows the guard block (TradeHandler.cpp:380-401) is
// a provable no-op for every representable deferred spell: under
// TRIGGERED_FULL_MASK the death, cooldown, GCD, battleground, shapeshift,
// aura-state, vehicle, power/reagent, caster-aura and dispel gates are all
// skipped (Spell.cpp:5174-5555, SpellDefines.h:134-153); the strict-only
// gates add nothing fireable for enchant spells; CheckExplicitTarget passes
// trivially (enchant ExplicitTargetMask = 0, SpellInfo.cpp:1782-1797); and
// the trade-slot check's only fireable term (m_CastItem ->
// SPELL_FAILED_ITEM_ENCHANT_TRADE_WINDOW, Spell.cpp:6167-6171) is
// unreachable at accept because CheckCast(false) already rejected any
// cast-item trade enchant at cast time — so a deferred spell always has a
// null cast item here, the slot sentinel is always TRADE_SLOT_NONTRADED
// (SetTradeItemTarget/Update, Spell.cpp:337-344/460-474), and the
// ITEM_ALREADY_ENCHANTED term is gated on !IsTriggered().
func (caster *session) tradeSpellStillCastable(ctx context.Context, target *session) bool {
	if caster.trade == nil || target.trade == nil {
		return false
	}
	if caster.server == nil || caster.server.Data == nil {
		return false
	}
	if _, found, err := caster.server.Data.Spell(caster.trade.SpellID); err != nil || !found {
		return false
	}
	if _, ok := target.trade.Items[tradeSlotNonTraded]; !ok {
		return false
	}
	if castItemGUID := caster.trade.SpellCastItemGUID; castItemGUID != 0 {
		cdb := caster.server.CharactersStore.DB
		if cdb == nil {
			return false
		}
		var one int64
		if err := cdb.QueryRowContext(ctx, "SELECT 1 FROM character_inventory WHERE guid = ? AND item = ? LIMIT 1", caster.playerGUID, castItemGUID).Scan(&one); err != nil {
			return false
		}
	}
	return true
}

// applyDeferredTradeEnchant mirrors the my_spell/his_spell->prepare at trade
// execute (TradeHandler.cpp:524-526) for the representable slices:
// SPELL_EFFECT_ENCHANT_ITEM (53, Spell::EffectEnchantItemPerm, SpellEffects.
// cpp:2704) writes the effect's MiscValue enchant ID into the target item's
// PERM_ENCHANTMENT_SLOT (ItemDefines.h:146: slot 0, index 0 of the 36-int
// enchantments column; duration/charges triplets zeroed like
// Item::SetEnchantment(slot, id, 0, 0, caster));
// SPELL_EFFECT_ENCHANT_ITEM_TEMPORARY (54, Spell::EffectEnchantItemTmp,
// SpellEffects.cpp:2832: enchant_id = Effects[effIndex].MiscValue,
// sSpellItemEnchantmentStore.LookupEntry, Item::SetEnchantment(TEMP_
// ENCHANTMENT_SLOT, enchant_id, duration * 1000, 0, casterGUID)) writes into
// TEMP_ENCHANTMENT_SLOT (ItemDefines.h:147: slot 1, index 3 of the column)
// with the C++ duration selection (SpellEffects.cpp:2913-2945) in
// milliseconds; SPELL_EFFECT_ENCHANT_ITEM_PRISMATIC (156,
// Spell::EffectEnchantItemPrismatic, SpellEffects.cpp:2768:
// Item::SetEnchantment(PRISMATIC_ENCHANTMENT_SLOT, enchantId, 0, 0, caster))
// writes into PRISMATIC_ENCHANTMENT_SLOT (ItemDefines.h:152: slot 6, index
// 18), only when the enchant entry carries an ITEM_ENCHANTMENT_TYPE_
// PRISMATIC_SOCKET (=8, DBCEnums.h) effect (SpellEffects.cpp:2788-2804).
// Both handlers carry the "item can be in trade slot and have owner diff.
// from caster" comment (SpellEffects.cpp:2908/2843), so the trade-window
// flow is evidenced for 54 and 156, not just 53. The spell target is the
// other party's non-traded slot item (SpellCastTargets::Update,
// Spell.cpp:470-474: the trade-item target resolves from the trader's
// TradeData), so the caller passes the target session the same way as
// tradeSpellStillCastable. Each enchant entry is validated against the DBC
// (sSpellItemEnchantmentStore.LookupEntry). Stat application for an equipped
// target is covered by the syncEquipmentCache calls later in completeTrade,
// which read the enchantment column.
func (caster *session) applyDeferredTradeEnchant(ctx context.Context, tx *sql.Tx, target *session) {
	if caster.trade == nil || target.trade == nil || caster.trade.SpellID == 0 {
		return
	}
	if caster.server == nil || caster.server.Data == nil {
		return
	}
	item, ok := target.trade.Items[tradeSlotNonTraded]
	if !ok {
		return
	}
	spell, found, err := caster.server.Data.Spell(caster.trade.SpellID)
	if err != nil || !found {
		return
	}
	type slotWrite struct {
		slot     uint32
		enchant  uint32
		duration uint32
	}
	var writes []slotWrite
	for _, eff := range spell.Effects {
		if eff.MiscValue <= 0 {
			continue
		}
		enchantID := uint32(eff.MiscValue)
		entry, ok, err := caster.server.Data.SpellItemEnchantment(enchantID)
		if err != nil || !ok {
			continue
		}
		switch eff.Effect {
		case 53: // SPELL_EFFECT_ENCHANT_ITEM
			writes = append(writes, slotWrite{slot: 0, enchant: enchantID})
		case 54: // SPELL_EFFECT_ENCHANT_ITEM_TEMPORARY
			// Rockbiter Weapon (SpellEffects.cpp:2843-2871): C++ enchants the
			// caster's own weapons via triggered spells and returns without
			// touching the item target, so a deferred Rockbiter writes
			// nothing to the trade item.
			if spell.SpellFamilyName == spellFamilyShaman && spell.SpellFamilyFlags[0]&0x400000 != 0 {
				continue
			}
			writes = append(writes, slotWrite{slot: 1, enchant: enchantID, duration: tempTradeEnchantDurationMs(spell)})
		case 156: // SPELL_EFFECT_ENCHANT_ITEM_PRISMATIC
			for _, e := range entry.Effects {
				if e == 8 { // ITEM_ENCHANTMENT_TYPE_PRISMATIC_SOCKET
					writes = append(writes, slotWrite{slot: 6, enchant: enchantID})
					break
				}
			}
		}
	}
	if len(writes) == 0 {
		return
	}
	var encStr sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT enchantments FROM item_instance WHERE guid = ? LIMIT 1", item.ItemGUID).Scan(&encStr); err != nil {
		return
	}
	fields := strings.Fields(encStr.String)
	var enchants [36]uint32
	for i := 0; i < len(fields) && i < 36; i++ {
		if val, err := strconv.ParseUint(fields[i], 10, 32); err == nil {
			enchants[i] = uint32(val)
		}
	}
	for _, w := range writes {
		base := w.slot * 3
		enchants[base] = w.enchant
		enchants[base+1] = w.duration
		enchants[base+2] = 0
	}
	encParts := make([]string, 36)
	for i := 0; i < 36; i++ {
		encParts[i] = strconv.FormatUint(uint64(enchants[i]), 10)
	}
	_, _ = tx.ExecContext(ctx, "UPDATE item_instance SET enchantments = ? WHERE guid = ?", strings.Join(encParts, " "), item.ItemGUID)
}

// tempTradeEnchantDurationMs mirrors the duration selection in
// Spell::EffectEnchantItemTmp (SpellEffects.cpp:2913-2945), converted to
// milliseconds the way C++ passes duration * 1000 to Item::SetEnchantment.
// The shaman Rockbiter special-case (SpellFamilyName == SHAMAN and
// SpellFamilyFlags[0] & 0x400000) has no model here: it enchants the
// caster's own equipped weapons via triggered spells and returns without
// touching the item target, so a deferred Rockbiter never writes to the
// trade item in C++ either.
func tempTradeEnchantDurationMs(spell wotlk.Spell) uint32 {
	var seconds uint32
	switch {
	case spell.ID == 38615:
		seconds = 1800
	case spell.SpellFamilyName == spellFamilyRogue:
		seconds = 3600
	case spell.SpellFamilyName == spellFamilyShaman:
		seconds = 1800
	case spell.SpellVisual[0] == 215:
		seconds = 1800
	case spell.SpellVisual[0] == 563 && spell.ID != 64401:
		seconds = 600
	case spell.SpellVisual[0] == 0:
		seconds = 1800
	case spell.ID == 29702:
		seconds = 300
	case spell.ID == 37360:
		seconds = 300
	default:
		seconds = 3600
	}
	return seconds * 1000
}

// sendTradeStatus sends SMSG_TRADE_STATUS (0x120) with matching TrinityCore structure.
// Reference: WorldSession::SendTradeStatus (TradeHandler.cpp:34).
func (s *session) sendTradeStatus(status uint32, traderGUID uint64, result uint32, isTargetResult uint8, itemLimitCategory uint32, slot uint8) error {
	buf := protocol.NewBuffer(16)
	buf.WriteU32(status)
	switch status {
	case tradeStatusBeginTrade:
		buf.WriteU64(traderGUID)
	case tradeStatusOpenWindow:
		buf.WriteU32(0) // tradeID
	case tradeStatusCloseWindow:
		buf.WriteU32(result)
		buf.WriteU8(isTargetResult)
		buf.WriteU32(itemLimitCategory)
	case tradeStatusWrongRealm, tradeStatusNotOnTaplist:
		buf.WriteU8(slot) // Trade slot; -1 here clears CGTradeInfo::m_tradeMoney
	}
	return s.write(uint16(protocol.OpcodeSMSG_TRADE_STATUS), buf.Bytes(), true)
}

// handleInitiateTrade processes CMSG_INITIATE_TRADE (0x116).
// Reference: WorldSession::HandleInitiateTradeOpcode (TradeHandler.cpp:590).
func (s *session) handleInitiateTrade(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	reader := protocol.NewReader(payload)
	targetGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	if s.trade != nil {
		return true
	}
	if s.player.MaxHealth > 0 && s.player.Health == 0 {
		_ = s.sendTradeStatus(tradeStatusYouDead, 0, 0, 0, 0, 0)
		return true
	}
	// Reference: WorldSession::HandleInitiateTradeOpcode (TradeHandler.cpp:606-610):
	// UNIT_STATE_STUNNED on the initiator answers TRADE_STATUS_YOU_STUNNED.
	// Go's model is the stun aura itself (conditions.go:625 — the same
	// HandleAuraModStun arm that sets UNIT_STATE_STUNNED in C++).
	if s.hasAuraType(spellAuraModStun) {
		_ = s.sendTradeStatus(tradeStatusYouStunned, 0, 0, 0, 0, 0)
		return true
	}
	// Reference: TradeHandler.cpp:612-616 — WorldSession::isLogingOut()
	// (_logoutTime set by CMSG_LOGOUT_REQUEST) answers TRADE_STATUS_YOU_LOGOUT.
	// logoutAt is Go's pending-logout-timer model (characters.go:2730).
	if !s.logoutAt.IsZero() {
		_ = s.sendTradeStatus(tradeStatusYouLogout, 0, 0, 0, 0, 0)
		return true
	}
	// Reference: TradeHandler.cpp:618-622 — an in-flight initiator answers
	// TRADE_STATUS_TARGET_TO_FAR (C++ reports the far arm here, not a
	// dedicated taxi status). isInFlight mirrors UNIT_STATE_IN_FLIGHT
	// (taxi.go:561).
	if s.isInFlight() {
		_ = s.sendTradeStatus(tradeStatusTargetTooFar, 0, 0, 0, 0, 0)
		return true
	}
	// Reference: WorldSession::HandleInitiateTradeOpcode (TradeHandler.cpp:624-631):
	// level below CONFIG_TRADE_LEVEL_REQ ("LevelReq.Trade", default 1) ->
	// notification + TRADE_STATUS_CLOSE_WINDOW before the target is even looked up.
	if uint32(s.player.Level) < s.server.Config.TradeLevelReq {
		// Notification text is not source-verifiable here: C++ sends
		// GetTrinityString(LANG_TRADE_REQ) (entry 6609) and the trinity_string
		// content lives in the DB import, not in this repo.
		s.sendNotification(fmt.Sprintf("You must be at least level %d to initiate a trade.", s.server.Config.TradeLevelReq))
		_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, 0, 0, 0, 0)
		return true
	}
	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil {
		_ = s.sendTradeStatus(tradeStatusNoTarget, 0, 0, 0, 0, 0)
		return true
	}
	if targetSess == s || targetSess.trade != nil {
		_ = s.sendTradeStatus(tradeStatusBusy, 0, 0, 0, 0, 0)
		return true
	}
	if targetSess.player.MaxHealth > 0 && targetSess.player.Health == 0 {
		_ = s.sendTradeStatus(tradeStatusTargetDead, 0, 0, 0, 0, 0)
		return true
	}
	// Reference: WorldSession::HandleInitiateTradeOpcode (TradeHandler.cpp:658-675):
	// target in-flight -> TARGET_TO_FAR, target stunned -> TARGET_STUNNED,
	// target logging out -> TARGET_LOGOUT, all before the ignore check.
	if targetSess.isInFlight() {
		_ = s.sendTradeStatus(tradeStatusTargetTooFar, 0, 0, 0, 0, 0)
		return true
	}
	if targetSess.hasAuraType(spellAuraModStun) {
		_ = s.sendTradeStatus(tradeStatusTargetStunned, 0, 0, 0, 0, 0)
		return true
	}
	if !targetSess.logoutAt.IsZero() {
		_ = s.sendTradeStatus(tradeStatusTargetLogout, 0, 0, 0, 0, 0)
		return true
	}
	// Reference: WorldSession::HandleInitiateTradeOpcode (TradeHandler.cpp:679-684):
	// the target ignoring the initiator answers TRADE_STATUS_IGNORE_YOU.
	if s.server.chatIgnoredBy(targetSess.playerGUID, s.playerGUID) {
		_ = s.sendTradeStatus(tradeStatusIgnoreYou, 0, 0, 0, 0, 0)
		return true
	}
	// Reference: WorldSession::HandleInitiateTradeOpcode (TradeHandler.cpp:686-693):
	// cross-faction trade is refused with TRADE_STATUS_WRONG_FACTION unless
	// CONFIG_ALLOW_TWO_SIDE_TRADE ("AllowTwoSide.Trade", default false) is set
	// or the session holds RBAC_PERM_ALLOW_TWO_SIDE_TRADE (id 51). C++ checks
	// this before the distance term. Go previously hard-blocked cross-faction
	// trade unconditionally.
	if s.player.Race != 0 && targetSess.player.Race != 0 && teamForRace(s.player.Race) != teamForRace(targetSess.player.Race) {
		twoSide := s.server.Config.AllowTwoSideTrade
		if !twoSide {
			granted, permErr := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionAllowTwoSideTrade)
			twoSide = permErr == nil && granted
		}
		if !twoSide {
			_ = s.sendTradeStatus(tradeStatusWrongFaction, 0, 0, 0, 0, 0)
			return true
		}
	}
	// Reference: TradeHandler.cpp:695-699 — IsWithinDistInMap(..., false) is a
	// 2D (X/Y only) check at TRADE_DISTANCE.
	if targetSess.player.Map != s.player.Map || distance2D(s.player.X, s.player.Y, targetSess.player.X, targetSess.player.Y) > tradeDistance {
		_ = s.sendTradeStatus(tradeStatusTargetTooFar, 0, 0, 0, 0, 0)
		return true
	}

	s.trade = &playerTradeState{Partner: targetSess, Items: make(map[uint8]tradeSlotItem)}
	targetSess.trade = &playerTradeState{Partner: s, Items: make(map[uint8]tradeSlotItem)}

	// Send SMSG_TRADE_STATUS (TRADE_STATUS_BEGIN_TRADE) to target
	_ = targetSess.sendTradeStatus(tradeStatusBeginTrade, s.playerGUID, 0, 0, 0, 0)
	s.debug("trade initiated", "from", s.accountName, "to", targetSess.accountName)
	return true
}

// handleBeginTrade processes CMSG_BEGIN_TRADE (0x117).
// Reference: WorldSession::HandleBeginTradeOpcode (TradeHandler.cpp:561).
func (s *session) handleBeginTrade(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || s.trade.Partner == nil {
		return true
	}
	partner := s.trade.Partner
	_ = s.sendTradeStatus(tradeStatusOpenWindow, 0, 0, 0, 0, 0)
	_ = partner.sendTradeStatus(tradeStatusOpenWindow, 0, 0, 0, 0, 0)
	s.notifyTradeUpdate()
	partner.notifyTradeUpdate()
	s.debug("trade window opened", "player1", s.accountName, "player2", partner.accountName)
	return true
}

// handleSetTradeGold processes CMSG_SET_TRADE_GOLD (0x11F).
// Reference: WorldSession::HandleSetTradeGoldOpcode (TradeHandler.cpp:711).
func (s *session) handleSetTradeGold(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || len(payload) < 4 {
		return true
	}
	reader := protocol.NewReader(payload)
	gold, err := reader.ReadU32()
	if err != nil {
		return false
	}
	// Reference: TradeData::SetMoney (TradeData.cpp:93-110). No-change is a
	// silent no-op (no un-accept, no update); insufficient funds answers
	// TRADE_STATUS_CLOSE_WINDOW + EQUIP_ERR_NOT_ENOUGH_MONEY and leaves the
	// stored money unchanged.
	if gold == s.trade.Money {
		return true
	}
	if gold > s.player.Money {
		_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, equipErrNotEnoughMoney, 0, 0, 0)
		return true
	}
	s.trade.Money = gold
	if s.trade.Accepted {
		s.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
	}
	if s.trade.Partner != nil && s.trade.Partner.trade != nil {
		if s.trade.Partner.trade.Accepted {
			s.trade.Partner.trade.Accepted = false
			_ = s.trade.Partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
		}
		s.notifyTradePartnerUpdate()
	}
	return true
}

// tradeHasItem answers whether the trade window already holds the given item
// GUID, matching TradeData::HasItem (TradeData.cpp:33) scanning all slots.
func tradeHasItem(items map[uint8]tradeSlotItem, itemGUID uint64) bool {
	for _, it := range items {
		if it.ItemGUID == itemGUID {
			return true
		}
	}
	return false
}

// handleSetTradeItem processes CMSG_SET_TRADE_ITEM (0x11D).
// Reference: WorldSession::HandleSetTradeItemOpcode (TradeHandler.cpp:723).
func (s *session) handleSetTradeItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || len(payload) < 3 {
		return true
	}
	tradeSlot := payload[0]
	bag := payload[1]
	slot := payload[2]
	if tradeSlot >= tradeSlotCount {
		// Invalid trade slot: C++ answers TRADE_STATUS_TRADE_CANCELED (TradeHandler.cpp:749-755).
		_ = s.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0, 0)
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var itemGUID int64
	err := cdb.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ? LIMIT 1", s.playerGUID, bag, slot).Scan(&itemGUID)
	if err != nil || itemGUID == 0 {
		// Missing item (cheating, can't fail with correct client operations): C++ answers
		// TRADE_STATUS_TRADE_CANCELED (TradeHandler.cpp:760-765).
		_ = s.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0, 0)
		return true
	}
	if tradeHasItem(s.trade.Items, uint64(itemGUID)) {
		// Prevent placing a single item into multiple trade slots (cheating attempt).
		_ = s.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0, 0)
		return true
	}
	var itemEntry, count, flags int64
	var giftCreatorGUID, creatorGUID uint64
	var durability, randomPropertyID int64
	var encStr sql.NullString
	_ = cdb.QueryRowContext(ctx, "SELECT itemEntry, count, flags, giftCreatorGuid, creatorGuid, enchantments, durability, randomPropertyId FROM item_instance WHERE guid = ? LIMIT 1", itemGUID).Scan(&itemEntry, &count, &flags, &giftCreatorGUID, &creatorGUID, &encStr, &durability, &randomPropertyID)
	if tradeSlot < tradeSlotTradedCount && (flags&1 != 0) {
		// Soulbound items cannot be placed in traded slots
		_ = s.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0, 0)
		return true
	}
	var enchantID uint32
	if encStr.Valid && encStr.String != "" {
		fields := strings.Fields(encStr.String)
		if len(fields) > 0 {
			if e, err := strconv.ParseUint(fields[0], 10, 32); err == nil {
				enchantID = uint32(e)
			}
		}
	}
	var displayID, lockID, maxDurability uint32
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var disp, lock, maxDur int64
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT displayid, lockid, MaxDurability FROM item_template WHERE entry = ?", itemEntry).Scan(&disp, &lock, &maxDur)
		displayID = uint32(disp)
		lockID = uint32(lock)
		maxDurability = uint32(maxDur)
	}
	if existing, ok := s.trade.Items[tradeSlot]; ok && existing.ItemGUID == uint64(itemGUID) {
		// TradeData::SetItem (TradeData.cpp:60-65) early-returns when the slot
		// already holds this item: no un-accept, no spell clear, no update.
		return true
	}
	// Reference: SendUpdateTrade (TradeHandler.cpp:95-96) writes
	// item->IsWrapped() ? 1 : 0. Go models wrapping via the character_gifts
	// row (items.go:2178), so the bit is derived from that row's presence.
	var wrappedProbe int64
	wrapped := cdb.QueryRowContext(ctx, "SELECT 1 FROM character_gifts WHERE item_guid = ? LIMIT 1", itemGUID).Scan(&wrappedProbe) == nil
	s.trade.Items[tradeSlot] = tradeSlotItem{
		ItemGUID:         uint64(itemGUID),
		ItemEntry:        uint32(itemEntry),
		DisplayID:        displayID,
		StackCount:       uint32(count),
		EnchantID:        enchantID,
		GiftCreatorGUID:  giftCreatorGUID,
		CreatorGUID:      creatorGUID,
		Wrapped:          wrapped,
		RandomPropertyID: uint32(randomPropertyID),
		Durability:       uint32(durability),
		MaxDurability:    maxDurability,
		LockID:           lockID,
	}
	// TradeData::SetItem spell clearing (TradeData.cpp:72-77): changing the
	// non-traded slot removes a possible spell the trader applied to it;
	// changing any slot removes a possible spell the player applied (the
	// reagent may have moved).
	if tradeSlot == tradeSlotNonTraded && s.trade.Partner != nil {
		s.trade.Partner.setTradeSpell(0, 0)
	}
	s.setTradeSpell(0, 0)
	if s.trade.Accepted {
		s.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
	}
	if s.trade.Partner != nil && s.trade.Partner.trade != nil {
		if s.trade.Partner.trade.Accepted {
			s.trade.Partner.trade.Accepted = false
			_ = s.trade.Partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
		}
		s.notifyTradePartnerUpdate()
	}
	return true
}

// handleClearTradeItem processes CMSG_CLEAR_TRADE_ITEM (0x11E).
// Reference: WorldSession::HandleClearTradeItemOpcode (TradeHandler.cpp:780).
func (s *session) handleClearTradeItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || len(payload) < 1 {
		return true
	}
	tradeSlot := payload[0]
	// Reference: WorldSession::HandleClearTradeItemOpcode (TradeHandler.cpp:784-793)
	// -> TradeData::SetItem(slot, nullptr) (TradeData.cpp:60-78): an invalid
	// slot returns silently, and clearing an already-empty slot early-returns
	// (no un-accept, no spell clear, no update).
	if tradeSlot >= tradeSlotCount {
		return true
	}
	if _, ok := s.trade.Items[tradeSlot]; !ok {
		return true
	}
	delete(s.trade.Items, tradeSlot)
	// HandleClearTradeItemOpcode routes through TradeData::SetItem(slot,
	// nullptr) (TradeHandler.cpp:778-792), so the same spell clearing as
	// handleSetTradeItem applies (TradeData.cpp:72-77).
	if tradeSlot == tradeSlotNonTraded && s.trade.Partner != nil {
		s.trade.Partner.setTradeSpell(0, 0)
	}
	s.setTradeSpell(0, 0)
	if s.trade.Accepted {
		s.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
	}
	if s.trade.Partner != nil && s.trade.Partner.trade != nil {
		if s.trade.Partner.trade.Accepted {
			s.trade.Partner.trade.Accepted = false
			_ = s.trade.Partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
		}
		s.notifyTradePartnerUpdate()
	}
	return true
}

// handleAcceptTrade processes CMSG_ACCEPT_TRADE (0x11A).
// Reference: WorldSession::HandleAcceptTradeOpcode (TradeHandler.cpp:337).
func (s *session) handleAcceptTrade(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || s.trade.Partner == nil {
		return true
	}
	partner := s.trade.Partner

	// Set before the checks so each failure can properly undo it, in C++ order
	// (TradeHandler.cpp:266-268).
	s.trade.Accepted = true

	// Accept-time distance check, in C++ position (TradeHandler.cpp:270-277):
	// the acceptor alone is answered TARGET_TO_FAR, their accept is undone
	// with a BACK_TO_TRADE notice to themselves (TradeData::SetAccepted(false)
	// with forTrader=false -> the owner's session), and the trade window stays
	// open — the partner is not notified and neither side tears down.
	// C++ uses IsWithinDistInMap(..., false): 2D at TRADE_DISTANCE.
	if partner.player == nil || s.player.Map != partner.player.Map || distance2D(s.player.X, s.player.Y, partner.player.X, partner.player.Y) > tradeDistance {
		_ = s.sendTradeStatus(tradeStatusTargetTooFar, 0, 0, 0, 0, 0)
		s.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
		return true
	}

	// Accept-time money guards, in C++ order (TradeHandler.cpp:278-306): each
	// failure un-accepts the failing side with a BACK_TO_TRADE notice to the
	// other party and keeps the trade window open.
	if s.player.Money < s.trade.Money {
		_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, equipErrNotEnoughMoney, 0, 0, 0)
		s.trade.Accepted = false
		_ = partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
		return true
	}
	if partner.trade != nil && partner.player.Money < partner.trade.Money {
		_ = partner.sendTradeStatus(tradeStatusCloseWindow, 0, equipErrNotEnoughMoney, 0, 0, 0)
		partner.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
		return true
	}
	if partner.trade != nil && s.player.Money >= maxMoneyAmount-partner.trade.Money {
		_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, equipErrTooMuchGold, 0, 0, 0)
		s.trade.Accepted = false
		_ = partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
		return true
	}
	if partner.trade != nil && partner.player.Money >= maxMoneyAmount-s.trade.Money {
		_ = partner.sendTradeStatus(tradeStatusCloseWindow, 0, equipErrTooMuchGold, 0, 0, 0)
		partner.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
		return true
	}

	// Accept-time item re-validation, in C++ order (TradeHandler.cpp:307-348):
	// every traded-slot item of both parties is re-checked with
	// Item::CanBeTraded(false, true) before the partner is notified. The only
	// representable term in Go's DB model is the soulbound flag (flags&1) —
	// BoP-tradeable state, non-empty bags, loot generation, CanUnequipItem,
	// loot GUIDs and enchant binding have no Go model, and the his-items
	// IsBindedNotWith branch is commented out in C++ too. A soulbound item
	// that reached a traded slot fails CanBeTraded, so the acceptor is
	// answered TRADE_STATUS_TRADE_CANCELED and the handler returns, leaving
	// the accepted state untouched exactly like C++. The CLOSE_WINDOW +
	// EQUIP_ERR_CANNOT_TRADE_THAT IsBindedNotWith branch is unreachable here:
	// with no BoP-tradeable model any soulbound item already fails the
	// CanBeTraded check first, and a non-soulbound item never binds.
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		for _, tr := range []*playerTradeState{s.trade, partner.trade} {
			if tr == nil {
				continue
			}
			for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
				it, ok := tr.Items[slot]
				if !ok {
					continue
				}
				var flags int64
				_ = cdb.QueryRowContext(ctx, "SELECT flags FROM item_instance WHERE guid = ? LIMIT 1", it.ItemGUID).Scan(&flags)
				if flags&1 != 0 {
					_ = s.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0, 0)
					return true
				}
			}
		}
	}

	// Inform partner
	_ = partner.sendTradeStatus(tradeStatusTradeAccept, 0, 0, 0, 0, 0)

	if partner.trade != nil && partner.trade.Accepted {
		// Both accepted -> enter the accept process (TradeHandler.cpp:356-357)
		// before executing: flags both trades and locks the traded items.
		setAcceptTradeMode(s, partner)

		// Deferred-spell accept-time re-validation (TradeHandler.cpp:364-438:
		// "not accept if spell can't be cast now (cheating)"). A stored spell
		// whose entry, target item, or cast item can no longer be resolved
		// leaves the accept process (clearAcceptTradeMode both overloads)
		// and is cleared (SetSpell(0)) in C++ order; the accept aborts and
		// the window stays open. The CheckCast(true) + SendCastResult spell
		// object validation has no Go model (no Spell object); the
		// my_spell/his_spell prepare at execute is modeled by
		// applyDeferredTradeEnchant for the SPELL_EFFECT_ENCHANT_ITEM slice.
		if s.trade.SpellID != 0 && !s.tradeSpellStillCastable(ctx, partner) {
			clearAcceptTradeMode(s, partner)
			s.setTradeSpell(0, 0)
			return true
		}
		if partner.trade.SpellID != 0 && !partner.tradeSpellStillCastable(ctx, s) {
			clearAcceptTradeMode(s, partner)
			partner.setTradeSpell(0, 0)
			return true
		}

		s.completeTrade(ctx, partner)
	}
	return true
}

type tradeSlotLoc struct {
	bagKey int64
	slot   uint8
}

func (s *session) findFreeSlotsForTrade(ctx context.Context, count int) ([]tradeSlotLoc, bool) {
	if count == 0 {
		return nil, true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return nil, false
	}
	var freeLocs []tradeSlotLoc

	// 1. Check backpack (bag = 0, slots 23..38)
	usedBackpack := make(map[uint8]bool)
	// Items locked in the trade are about to leave this inventory: skip
	// them in the occupancy scan (Player::CanStoreItems skips IsInTrade
	// items when building the fit model, Player.cpp:11157).
	locked := s.trade != nil && len(s.trade.InTradeItems) > 0
	rows, err := cdb.QueryContext(ctx, "SELECT slot, item FROM character_inventory WHERE guid = ? AND bag = 0", s.playerGUID)
	if err == nil {
		for rows.Next() {
			var sl, it int64
			if rows.Scan(&sl, &it) == nil {
				if locked && s.trade.InTradeItems[uint64(it)] {
					continue
				}
				usedBackpack[uint8(sl)] = true
			}
		}
		rows.Close()
	}
	for sl := uint8(23); sl <= 38; sl++ {
		if !usedBackpack[sl] {
			freeLocs = append(freeLocs, tradeSlotLoc{bagKey: 0, slot: sl})
			if len(freeLocs) == count {
				return freeLocs, true
			}
		}
	}

	// 2. Check equipped bags (slots 19..22)
	equippedBags := s.getEquippedBags(ctx, s.playerGUID)
	for _, eb := range equippedBags {
		maxSlots := eb.slots
		if maxSlots <= 0 || maxSlots > 36 {
			continue
		}
		usedBagSlots := make(map[uint8]bool)
		bRows, bErr := cdb.QueryContext(ctx, "SELECT slot, item FROM character_inventory WHERE guid = ? AND bag = ?", s.playerGUID, eb.guid)
		if bErr == nil {
			for bRows.Next() {
				var bsl, bit int64
				if bRows.Scan(&bsl, &bit) == nil {
					if locked && s.trade.InTradeItems[uint64(bit)] {
						continue
					}
					usedBagSlots[uint8(bsl)] = true
				}
			}
			bRows.Close()
		}
		for bsl := uint8(0); bsl < uint8(maxSlots); bsl++ {
			if !usedBagSlots[bsl] {
				freeLocs = append(freeLocs, tradeSlotLoc{bagKey: eb.guid, slot: bsl})
				if len(freeLocs) == count {
					return freeLocs, true
				}
			}
		}
	}

	return freeLocs, len(freeLocs) >= count
}

// completeTrade finalizes the trade, exchanging traded items (slots 0..5) and currency.
// Reference: WorldSession::HandleAcceptTradeOpcode (TradeHandler.cpp:443-544).
func (s *session) completeTrade(ctx context.Context, partner *session) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		clearAcceptTradeMode(s, partner)
		return
	}

	// Only slots 0..5 (TRADE_SLOT_TRADED_COUNT = 6) are traded; slot 6 is non-traded
	var sTradedItems []tradeSlotItem
	for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
		if it, ok := s.trade.Items[slot]; ok {
			sTradedItems = append(sTradedItems, it)
		}
	}
	var partnerTradedItems []tradeSlotItem
	for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
		if it, ok := partner.trade.Items[slot]; ok {
			partnerTradedItems = append(partnerTradedItems, it)
		}
	}

	partnerSlots, ok1 := partner.findFreeSlotsForTrade(ctx, len(sTradedItems))
	sSlots, ok2 := s.findFreeSlotsForTrade(ctx, len(partnerTradedItems))
	if !ok1 || !ok2 {
		// Fit failure (TradeHandler.cpp:449-476): leave the accept process
		// before answering CLOSE_WINDOW, in C++ order. C++ checks the
		// acceptor's own fit FIRST (the myCanCompleteInfo arm): the failing
		// acceptor is answered second with IsTargetResult set, after the
		// partner; the partner-fit arm (hisCanCompleteInfo) answers the
		// acceptor first, then the failing partner with IsTargetResult set.
		// Both arms un-accept both sides and KEEP the trade alive — the
		// client windows stay open so the players can make space and
		// re-accept. The delete my_spell/delete his_spell deletes the local
		// Spell objects only; Go stores no Spell object (SpellID is the
		// representable term), so there is nothing to clear here.
		clearAcceptTradeMode(s, partner)
		if !ok2 {
			_ = partner.sendTradeStatus(tradeStatusCloseWindow, 0, equipErrBagFull, 0, 0, 0)
			_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, equipErrBagFull, 1, 0, 0) // isTargetResult = 1
		} else {
			_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, equipErrBagFull, 0, 0, 0)
			_ = partner.sendTradeStatus(tradeStatusCloseWindow, 0, equipErrBagFull, 1, 0, 0) // isTargetResult = 1
		}
		s.trade.Accepted = false
		partner.trade.Accepted = false
		return
	}

	if s.player.Money < s.trade.Money || partner.player.Money < partner.trade.Money {
		clearAcceptTradeMode(s, partner)
		_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, 0, 0, 0, 0)
		_ = partner.sendTradeStatus(tradeStatusCloseWindow, 0, 0, 0, 0, 0)
		s.trade = nil
		partner.trade = nil
		return
	}

	// Execute trade inside one transaction: C++ wraps both players'
	// SaveInventoryAndGoldToDB in a single CharacterDatabaseTransaction
	// (TradeHandler.cpp:532-535). A mid-trade DB failure rolls back, tears
	// the trade down, and closes both client windows with TRADE_CANCELED
	// (C++ has no failure arm here; leaving the windows open on torn-down
	// server state would desync the clients).
	tx, err := cdb.BeginTx(ctx, nil)
	if err != nil {
		clearAcceptTradeMode(s, partner)
		s.trade = nil
		partner.trade = nil
		return
	}
	aborted := false
	abort := func() {
		if !aborted {
			aborted = true
			_ = tx.Rollback()
			clearAcceptTradeMode(s, partner)
			_ = s.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0, 0)
			_ = partner.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0, 0)
			s.trade = nil
			partner.trade = nil
		}
	}
	execTx := func(query string, args ...interface{}) bool {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			abort()
			return false
		}
		return true
	}

	// Money transfer
	s.player.Money = s.player.Money - s.trade.Money + partner.trade.Money
	partner.player.Money = partner.player.Money - partner.trade.Money + s.trade.Money
	if !execTx("UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID) {
		return
	}
	if !execTx("UPDATE characters SET money = ? WHERE guid = ?", partner.player.Money, partner.playerGUID) {
		return
	}

	sTransferSlots := make(map[uint8]int, tradeSlotTradedCount)
	partnerTransferSlots := make(map[uint8]int, tradeSlotTradedCount)
	for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
		if it, ok := s.trade.Items[slot]; ok {
			sTransferSlots[slot] = len(sTransferSlots)
			if !execTx("DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, it.ItemGUID) {
				return
			}
			s.despawnItem(it.ItemGUID)
			s.adjustQuestItemCount(ctx, it.ItemEntry, it.StackCount, false)
		}
		if it, ok := partner.trade.Items[slot]; ok {
			partnerTransferSlots[slot] = len(partnerTransferSlots)
			if !execTx("DELETE FROM character_inventory WHERE guid = ? AND item = ?", partner.playerGUID, it.ItemGUID) {
				return
			}
			partner.despawnItem(it.ItemGUID)
			partner.adjustQuestItemCount(ctx, it.ItemEntry, it.StackCount, false)
		}
	}

	for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
		if it, ok := s.trade.Items[slot]; ok {
			targetLoc := partnerSlots[sTransferSlots[slot]]
			// Execute trade: C++ stamps the giver's GUID as ITEM_FIELD_GIFTCREATOR
			// on each traded item (TradeHandler.cpp:483).
			if !execTx("UPDATE item_instance SET owner_guid = ?, giftCreatorGuid = ? WHERE guid = ?", partner.playerGUID, s.playerGUID, it.ItemGUID) {
				return
			}
			if !execTx("INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", partner.playerGUID, targetLoc.bagKey, targetLoc.slot, it.ItemGUID) {
				return
			}
			partner.adjustQuestItemCount(ctx, it.ItemEntry, it.StackCount, true)
		}
		if it, ok := partner.trade.Items[slot]; ok {
			targetLoc := sSlots[partnerTransferSlots[slot]]
			// Execute trade: C++ stamps the giver's GUID as ITEM_FIELD_GIFTCREATOR
			// on each traded item (TradeHandler.cpp:488).
			if !execTx("UPDATE item_instance SET owner_guid = ?, giftCreatorGuid = ? WHERE guid = ?", s.playerGUID, partner.playerGUID, it.ItemGUID) {
				return
			}
			if !execTx("INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", s.playerGUID, targetLoc.bagKey, targetLoc.slot, it.ItemGUID) {
				return
			}
			s.adjustQuestItemCount(ctx, it.ItemEntry, it.StackCount, true)
		}
	}

	// Deferred-spell prepare at execute (TradeHandler.cpp:524-526): each
	// side's deferred spell is applied to the other side's non-traded slot
	// item now that the inventory move is done, before the trade completes.
	s.applyDeferredTradeEnchant(ctx, tx, partner)
	partner.applyDeferredTradeEnchant(ctx, tx, s)

	if err := tx.Commit(); err != nil {
		abort()
		return
	}

	_ = s.sendTradeStatus(tradeStatusTradeComplete, 0, 0, 0, 0, 0)
	_ = partner.sendTradeStatus(tradeStatusTradeComplete, 0, 0, 0, 0, 0)

	_ = s.sendInventoryItems(ctx)
	_ = partner.sendInventoryItems(ctx)
	s.sendPlayerMoneyUpdate()
	partner.sendPlayerMoneyUpdate()
	s.syncEquipmentCache(ctx)
	partner.syncEquipmentCache(ctx)
	s.sendPlayerUpdate()
	partner.sendPlayerUpdate()

	// Cleanup: leave the accept process (TradeHandler.cpp:529) before the
	// trade state is torn down.
	clearAcceptTradeMode(s, partner)
	s.trade = nil
	partner.trade = nil
	s.debug("trade completed successfully", "player1", s.accountName, "player2", partner.accountName)
}

// handleUnacceptTrade processes CMSG_UNACCEPT_TRADE (0x11B).
// Reference: WorldSession::HandleUnacceptTradeOpcode (TradeHandler.cpp:552).
func (s *session) handleUnacceptTrade(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil {
		return true
	}
	s.trade.Accepted = false
	// Reference: WorldSession::HandleUnacceptTradeOpcode (TradeHandler.cpp:552-558)
	// -> TradeData::SetAccepted(false, forTrader=true) (TradeData.cpp:122-133):
	// only the un-acceptor's own accepted state clears; TRADE_STATUS_BACK_TO_TRADE
	// goes to the trader (partner) alone. The partner's accepted flag is untouched.
	if s.trade.Partner != nil {
		_ = s.trade.Partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0, 0)
	}
	return true
}

// handleCancelTrade processes CMSG_CANCEL_TRADE (0x11C).
// Reference: WorldSession::HandleCancelTradeOpcode (TradeHandler.cpp:583).
// cancelTrade mirrors Player::TradeCancel (Player.cpp:13709): tears the
// trade down and answers TRADE_STATUS_TRADE_CANCELED (== C++
// SendCancelTrade) to the partner; with sendback=true the cancelling
// player's own client is answered too. The CMSG_CANCEL_TRADE path calls
// with sendback=true; the taxi flight-start path (Player.cpp:21586) calls
// TradeCancel(true) unconditionally because the client closes its trade
// window when the taxi map opens but cheating tools can reopen it.
func (s *session) cancelTrade(sendback bool) {
	if s.trade == nil {
		return
	}
	partner := s.trade.Partner
	if sendback {
		_ = s.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0, 0)
	}
	s.trade = nil
	if partner != nil {
		partner.trade = nil
		_ = partner.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0, 0)
	}
}

func (s *session) handleCancelTrade(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil {
		return true
	}
	s.cancelTrade(true)
	return true
}

// sendTradeStatusExtended sends SMSG_TRADE_STATUS_EXTENDED (0x121).
// Reference: WorldSession::SendUpdateTrade (TradeHandler.cpp:75).
func (s *session) sendTradeStatusExtended(traderData bool) {
	if s.trade == nil || s.trade.Partner == nil {
		return
	}
	data := s.trade
	if traderData {
		data = s.trade.Partner.trade
	}
	if data == nil {
		return
	}
	buf := protocol.NewBuffer(256)
	if traderData {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	buf.WriteU32(0)              // tradeID
	buf.WriteU32(tradeSlotCount) // tradeSlotCount
	buf.WriteU32(tradeSlotCount) // tradeSlotCount
	buf.WriteU32(data.Money)
	buf.WriteU32(data.SpellID) // spell (TradeData::GetSpell, TradeHandler.cpp:85)
	for i := uint8(0); i < tradeSlotCount; i++ {
		buf.WriteU8(i)
		if it, ok := data.Items[i]; ok {
			buf.WriteU32(it.ItemEntry)
			buf.WriteU32(it.DisplayID)
			buf.WriteU32(it.StackCount)
			if it.Wrapped {
				buf.WriteU32(1) // wrapped: hide stats but show giftcreator name
			} else {
				buf.WriteU32(0)
			}
			buf.WriteU64(it.GiftCreatorGUID) // giftCreator (SendUpdateTrade, TradeHandler.cpp:98)
			buf.WriteU32(it.EnchantID)       // permEnchant
			for j := 0; j < 3; j++ {
				buf.WriteU32(0) // gem sockets
			}
			buf.WriteU64(it.CreatorGUID) // creator (SendUpdateTrade, TradeHandler.cpp:101)
			buf.WriteU32(0)              // charges: C++ item->GetSpellCharges(); Go's
			// item_instance.charges TEXT column is never populated with live
			// charge data, so there is nothing representable to send.
			buf.WriteU32(0) // suffixFactor: no Go model (no item_instance column)
			buf.WriteU32(it.RandomPropertyID)
			buf.WriteU32(it.LockID)
			buf.WriteU32(it.MaxDurability)
			buf.WriteU32(it.Durability)
		} else {
			for j := 0; j < 18; j++ {
				buf.WriteU32(0)
			}
		}
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_TRADE_STATUS_EXTENDED), buf.Bytes(), true)
}

func (s *session) notifyTradeUpdate() {
	s.sendTradeStatusExtended(false)
	if s.trade != nil && s.trade.Partner != nil {
		s.trade.Partner.sendTradeStatusExtended(true)
	}
}

// notifyTradePartnerUpdate mirrors TradeData::Update(true)
// (TradeData.cpp:119-125): only the trader's (partner's) session receives
// the extended update carrying the changer's data. The SetItem/SetMoney
// paths use this — C++ sends no extended update back to the changing
// player's own client there (SetSpell is the only path that updates both
// sides, and it keeps notifyTradeUpdate).
func (s *session) notifyTradePartnerUpdate() {
	if s.trade != nil && s.trade.Partner != nil {
		s.trade.Partner.sendTradeStatusExtended(true)
	}
}
