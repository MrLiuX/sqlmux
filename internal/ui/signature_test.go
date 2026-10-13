package ui

import "testing"

// The fields shown are the current one and those around it that fit with a …
// for each side that has more, the room counting them (F15.3).
func TestSignatureFit(t *testing.T) {
	s := Signature{Fields: []string{"aaaa", "bbbb", "cccc", "dddd", "eeee"}, Cur: 2}
	for _, c := range []struct{ room, from, to int }{
		{4, 2, 3},   // the current one alone, whatever the room
		{10, 2, 3},  // …, cccc, …
		{13, 2, 3},  // the next one with both … is 16
		{16, 2, 4},  // right first
		{22, 1, 4},  // then left
		{100, 0, 5}, // all, no …
	} {
		if from, to := s.fit(c.room); from != c.from || to != c.to {
			t.Errorf("room %d: fields %d to %d, want %d to %d", c.room, from, to, c.from, c.to)
		}
	}
}
