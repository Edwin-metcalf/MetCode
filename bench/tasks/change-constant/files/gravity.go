package fixture

import "math"

const gravity = 8.81

type Vector3 struct {
	X, Y, Z float64
}

func calculateGravity(pos1, pos2 Vector3, mass1, mass2 float64) Vector3 {
	distance := Vector3{
		X: pos2.X - pos1.X,
		Y: pos2.Y - pos1.Y,
		Z: pos2.Z - pos1.Z,
	}

	ds := distance.X*distance.X + distance.Y*distance.Y + distance.Z*distance.Z
	if ds == 0 {
		return Vector3{0, 0, 0} // Avoid division by zero
	}

	forceMag := gravity * (mass1 * mass2) / ds
	dirx := distance.X / math.Sqrt(ds)
	diry := distance.Y / math.Sqrt(ds)
	dirz := distance.Z / math.Sqrt(ds)

	return Vector3{
		X: forceMag * dirx,
		Y: forceMag * diry,
		Z: forceMag * dirz,
	}

}
