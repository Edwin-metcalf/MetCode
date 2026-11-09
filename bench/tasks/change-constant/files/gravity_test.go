package fixture

import (
	"math"
	"testing"
)

func TestCalculateGravity(t *testing.T) {
	const epsilon = 1e-6

	tests := []struct {
		name      string
		pos1      Vector3
		pos2      Vector3
		mass1     float64
		mass2     float64
		wantForce Vector3
	}{
		{
			name:  "Straight pull on X-Axis",
			pos1:  Vector3{X: 0, Y: 0, Z: 0},
			pos2:  Vector3{X: 2, Y: 0, Z: 0}, // Distance r = 2, rSq = 4
			mass1: 1.0,
			mass2: 2.0,
			// F = 9.81 * (1 * 2) / 4 = 4.905 pulling straight right (+X)
			wantForce: Vector3{X: 4.905, Y: 0, Z: 0},
		},
		{
			name:  "Diagonal pull in 3D",
			pos1:  Vector3{X: 0, Y: 0, Z: 0},
			pos2:  Vector3{X: 1, Y: 1, Z: 1}, // Distance r = sqrt(3)
			mass1: 1.0,
			mass2: 1.0,
			// F_mag = 9.81 * (1 * 1) / 3 = 3.27
			// Unit vector components = 1 / sqrt(3) ~= 0.57735
			// Component force ~= 3.27 * 0.57735 = 1.887932
			wantForce: Vector3{X: 1.887932, Y: 1.887932, Z: 1.887932},
		},
		{
			name:      "Zero distance division safety",
			pos1:      Vector3{X: 5, Y: 5, Z: 5},
			pos2:      Vector3{X: 5, Y: 5, Z: 5},
			mass1:     10.0,
			mass2:     20.0,
			wantForce: Vector3{X: 0, Y: 0, Z: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateGravity(tt.pos1, tt.pos2, tt.mass1, tt.mass2)

			// Helper function to check if float values are close enough
			isClose := func(a, b float64) bool {
				return math.Abs(a-b) < epsilon
			}

			if !isClose(got.X, tt.wantForce.X) || !isClose(got.Y, tt.wantForce.Y) || !isClose(got.Z, tt.wantForce.Z) {
				t.Errorf("CalculateGravitationalForce() = %+v, want %+v", got, tt.wantForce)
			}
		})
	}
}
