package fixture

import "testing"

func TestHighScore(t *testing.T) {
	cases := []struct {
		scores []int
		want   int
	}{
		{scores: []int{3, 9, 2}, want: 9},
		{scores: []int{1, 2, 9}, want: 9},
		{scores: []int{7}, want: 7},
		{scores: []int{-5, -2, -9}, want: -2},
	}
	for _, c := range cases {
		got := highScore(c.scores)
		if got != c.want {
			t.Errorf("highScore(%v) = %v; want %v", c.scores, got, c.want)
		}
	}
}
