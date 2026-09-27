package world

import "testing"

func TestCanPlayerCastSpell(t *testing.T) {
	for _, tc := range []struct {
		name    string
		learned bool
		gmMode  bool
		want    bool
	}{
		{name: "learned spell", learned: true, want: true},
		{name: "unlearned ordinary player", want: false},
		{name: "GM can cast unlearned spell", gmMode: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := canPlayerCastSpell(tc.learned, tc.gmMode); got != tc.want {
				t.Fatalf("canPlayerCastSpell() = %t, want %t", got, tc.want)
			}
		})
	}
}
