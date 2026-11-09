package fixture

func highScore(scores []int) int {
	best := 0
	for i := 0; i < len(scores)-1; i++ {
		if scores[i] > best {
			best = scores[i]
		}
	}
	return best
}
