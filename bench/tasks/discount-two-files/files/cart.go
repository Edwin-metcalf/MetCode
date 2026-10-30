package fixture

// Item is one line in a cart. Percent is the discount for that item (0-100).
type Item struct {
	PriceCents int
	Percent    int
}

// Total returns the sum of the discounted prices of all items, in cents.
func Total(items []Item) int {
	total := 0
	for _, it := range items {
		total += Discounted(it.PriceCents, it.Percent)
	}
	return total
}
