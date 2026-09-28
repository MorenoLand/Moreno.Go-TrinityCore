package world

const (
	itemHonorPointsID      uint32 = 43308
	itemArenaPointsID      uint32 = 43307
	currencyTokenSlotStart        = 118
	currencyTokenSlotEnd          = 150
)

func (s *session) addKnownCurrency(state *playerState, itemID uint32) bool {
	if s == nil || state == nil || s.server == nil || s.server.Data == nil || itemID == 0 {
		return false
	}
	bitIndex, found, err := s.server.Data.CurrencyBitIndex(itemID)
	if err != nil || !found || bitIndex == 0 || bitIndex > 64 {
		return false
	}
	mask := uint64(1) << (bitIndex - 1)
	if state.KnownCurrency&mask != 0 {
		return false
	}
	state.KnownCurrency |= mask
	return true
}
