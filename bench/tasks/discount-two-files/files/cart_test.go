package fixture

import "testing"

func TestTotal(t *testing.T) {
	items := []Item{
		{PriceCents: 1000, Percent: 10}, // 900
		{PriceCents: 2000, Percent: 25}, // 1500
	}
	if got, want := Total(items), 2400; got != want {
		t.Errorf("Total = %d, want %d", got, want)
	}
}

func TestTotalNoDiscount(t *testing.T) {
	items := []Item{{PriceCents: 500, Percent: 0}}
	if got, want := Total(items), 500; got != want {
		t.Errorf("Total = %d, want %d", got, want)
	}
}

func TestTotalEmptyCart(t *testing.T) {
	if got := Total(nil); got != 0 {
		t.Errorf("Total(nil) = %d, want 0", got)
	}
}
