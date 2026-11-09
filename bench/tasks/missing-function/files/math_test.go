package fixture

import (
	"reflect"
	"testing"
)

func TestMultiplyMatrices(t *testing.T) {
	t.Run("Test Matix Multiplication", func(t *testing.T) {
		matrixA := [][]int{
			{1, 2, 3},
			{4, 5, 6},
		}
		matrixB := [][]int{
			{7, 8},
			{9, 10},
		}

		got := multiplyMatrices(matrixA, matrixB)
		want := [][]int{
			{58, 64},
			{139, 154},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("multiplyMatrices() = %v, want %v", got, want)
		}
	})

	t.Run("Test Zero matrix multiplication", func(t *testing.T) {
		matrixA := [][]int{
			{0, 0},
			{0, 0},
		}
		matrixB := [][]int{
			{1, 2},
			{3, 4},
		}
		got := multiplyMatrices(matrixA, matrixB)
		want := [][]int{
			{0, 0},
			{0, 0},
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("multiplyMatrices() = %v, want %v", got, want)
		}
	})
}
