package fixture

import "testing"

func TestAdd(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{2, 3, 5},
		{0, 0, 0},
		{-1, 1, 0},
		{10, -4, 6},
	}
	for _, c := range cases {
		if got := Add(c.a, c.b); got != c.want {
			t.Errorf("Add(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
