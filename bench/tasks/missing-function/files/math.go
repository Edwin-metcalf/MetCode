package fixture

func dotProduct(a, b []int) float64 {
	var total float64
	for idx, _ := range a {
		total += float64(a[idx] * b[idx])
	}
	return total
}

func multiplyMatrices(a, b [][]int) [][]int {
	return [][]int{
		{1},
		{1},
	}
}
