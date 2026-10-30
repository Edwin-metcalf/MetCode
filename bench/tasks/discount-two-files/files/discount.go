package fixture

// Discounted returns priceCents reduced by percent (0-100).
func Discounted(priceCents, percent int) int {
	return priceCents - priceCents*percent/10
}
