package world

import (
	"context"
	"os"
	"testing"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func TestDeadPlayerSayReviveCommandRunsBeforeChatRestrictions(t *testing.T) {
	s := &session{server: &Server{}, playerLoaded: true, playerGUID: 1, player: &playerState{Health: 0, MaxHealth: 100, PlayerFlags: playerFlagGhost}}
	payload := protocol.NewBuffer(16)
	payload.WriteU32(chatSay)
	payload.WriteU32(0)
	payload.WriteCString(".rev")
	if !s.handleMessageChat(context.Background(), payload.Bytes()) {
		t.Fatal("chat handler rejected the command packet")
	}
	if s.player.Health != 100 || s.player.PlayerFlags&playerFlagGhost != 0 {
		t.Fatalf("dead Say command did not revive the player: health=%d flags=%#x", s.player.Health, s.player.PlayerFlags)
	}
}

func TestSpiritHealerResurrectAcceptUsesDBCRequestData(t *testing.T) {
	if _, err := os.Stat("bin/data/dbc/Spell.dbc"); err != nil {
		t.Skip("Spell.dbc is unavailable")
	}
	data := wotlk.NewStore("bin/data/dbc")
	spell, found, err := data.Spell(17251)
	if err != nil || !found {
		t.Fatalf("load spirit-healer spell 17251: found=%t err=%v", found, err)
	}
	var resurrect *wotlk.SpellEffect
	for i := range spell.Effects {
		if spell.Effects[i].Effect == spellEffectResurrectNew {
			resurrect = &spell.Effects[i]
			break
		}
	}
	if resurrect == nil {
		t.Fatal("spirit-healer spell has no resurrect-new effect")
	}
	healerGUID := uint64(0xF130000000001649)
	player := &playerState{Map: 0, X: 1819.41, Y: 219.233, Z: 60.0732, Health: 1, MaxHealth: 1000, Level: 80, PlayerFlags: playerFlagGhost}
	s := &session{server: &Server{Data: data}, playerLoaded: true, playerGUID: 2, player: player}
	s.requestSpiritHealerResurrection(healerGUID, "Spirit Healer", player.Map, player.X, player.Y, player.Z)
	if s.resurrection == nil || s.resurrection.GUID != healerGUID {
		t.Fatal("Spirit Healer did not create the pending resurrection request")
	}
	minHealth, maxHealth := resurrect.CalcValueRangeForLevel(spell, uint32(player.Level))
	response := protocol.NewBuffer(9)
	response.WriteU64(healerGUID)
	response.WriteU8(1)
	if !s.handleResurrectResponse(context.Background(), response.Bytes()) {
		t.Fatal("resurrection response was rejected")
	}
	if player.PlayerFlags&playerFlagGhost != 0 || int32(player.Health) < minHealth || int32(player.Health) > maxHealth || s.resurrection != nil {
		t.Fatalf("accepted resurrection did not apply request: health=%d flags=%#x pending=%t expected_health=%d..%d", player.Health, player.PlayerFlags, s.resurrection != nil, minHealth, maxHealth)
	}
}
