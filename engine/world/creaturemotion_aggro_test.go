package world

import "testing"

func TestCanCreatureStartAttackRespectsVerticalSeparation(t *testing.T) {
	ground := &creatureMotion{Z: 10}
	cases := []struct {
		name     string
		motion   *creatureMotion
		targetZ  float32
		distance float32
		want     bool
	}{
		{name: "ground limit included", motion: ground, targetZ: 13, distance: 3, want: true},
		{name: "different floor rejected", motion: ground, targetZ: 13.01, distance: 4, want: false},
		{name: "flying creature ignores vertical limit", motion: &creatureMotion{Z: 10, CanFly: true}, targetZ: 18, distance: 9, want: true},
		{name: "flying creature still obeys attack range", motion: &creatureMotion{Z: 10, CanFly: true}, targetZ: 30, distance: 21, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canCreatureStartAttack(tc.motion, playerPos{Z: tc.targetZ}, tc.distance, 15); got != tc.want {
				t.Fatalf("canCreatureStartAttack() = %t, want %t", got, tc.want)
			}
		})
	}
}
