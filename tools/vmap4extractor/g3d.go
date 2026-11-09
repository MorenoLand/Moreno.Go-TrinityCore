// G3D math needed by Doodad::ExtractSet (model.cpp), transcribed from the
// vendored G3D lite sources in ~/workspace/moreno-trinitycore/dep/g3dlite:
//   - Matrix3::fromEulerAnglesZYX (source/Matrix3.cpp:1664):
//     kZ(fYAngle) * (kY(fPAngle) * kX(fRAngle)), row-major elt[row][col],
//     matrix * vector is a column-vector product.
//   - Matrix3::Matrix3(const Quat&) (source/Matrix3.cpp:122): unitize the
//     quat, then the Watt & Watt formula.
//   - Matrix3::toEulerAnglesXYZ (source/Matrix3.cpp:1383), including both
//     singular branches.
//   - Matrix3::operator* (source/Matrix3.cpp:278): standard row x column.
//   - fixCoords (wmo.h): (z, x, y). fixCoordSystem (model.cpp): (x, z, -y).
package main

import "math"

type vec3 struct {
	X, Y, Z float32
}

func fixCoords(v vec3) vec3 { return vec3{v.Z, v.X, v.Y} }

func fixCoordSystem(v vec3) vec3 { return vec3{v.X, v.Z, -v.Y} }

func addVec3(a, b vec3) vec3 { return vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }

type mat3 struct {
	m [3][3]float32
}

func (a mat3) mul(b mat3) mat3 {
	var out mat3
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			out.m[r][c] = a.m[r][0]*b.m[0][c] + a.m[r][1]*b.m[1][c] + a.m[r][2]*b.m[2][c]
		}
	}
	return out
}

func (a mat3) mulVec(v vec3) vec3 {
	return vec3{
		a.m[0][0]*v.X + a.m[0][1]*v.Y + a.m[0][2]*v.Z,
		a.m[1][0]*v.X + a.m[1][1]*v.Y + a.m[1][2]*v.Z,
		a.m[2][0]*v.X + a.m[2][1]*v.Y + a.m[2][2]*v.Z,
	}
}

func rotX(angle float32) mat3 {
	c, s := float32(math.Cos(float64(angle))), float32(math.Sin(float64(angle)))
	return mat3{m: [3][3]float32{{1, 0, 0}, {0, c, -s}, {0, s, c}}}
}

func rotY(angle float32) mat3 {
	c, s := float32(math.Cos(float64(angle))), float32(math.Sin(float64(angle)))
	return mat3{m: [3][3]float32{{c, 0, s}, {0, 1, 0}, {-s, 0, c}}}
}

func rotZ(angle float32) mat3 {
	c, s := float32(math.Cos(float64(angle))), float32(math.Sin(float64(angle)))
	return mat3{m: [3][3]float32{{c, -s, 0}, {s, c, 0}, {0, 0, 1}}}
}

// fromEulerAnglesZYX mirrors G3D: the first argument rotates about Z, the
// second about Y, the third about X; result is kZ * (kY * kX). model.cpp
// calls it as fromEulerAnglesZYX(rot.y, rot.x, rot.z) in radians.
func fromEulerAnglesZYX(yAngle, pAngle, rAngle float32) mat3 {
	return rotZ(yAngle).mul(rotY(pAngle).mul(rotX(rAngle)))
}

type quat struct {
	X, Y, Z, W float32
}

func (q quat) unitize() quat {
	n := float32(math.Sqrt(float64(q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W)))
	if n == 0 {
		return quat{0, 0, 0, 1}
	}
	return quat{q.X / n, q.Y / n, q.Z / n, q.W / n}
}

// toRotationMatrix mirrors Matrix3::Matrix3(const Quat&).
func (q quat) toRotationMatrix() mat3 {
	q = q.unitize()
	xx := 2 * q.X * q.X
	xy := 2 * q.X * q.Y
	xz := 2 * q.X * q.Z
	xw := 2 * q.X * q.W
	yy := 2 * q.Y * q.Y
	yz := 2 * q.Y * q.Z
	yw := 2 * q.Y * q.W
	zz := 2 * q.Z * q.Z
	zw := 2 * q.Z * q.W
	return mat3{m: [3][3]float32{
		{1 - yy - zz, xy - zw, xz + yw},
		{xy + zw, 1 - xx - zz, yz - xw},
		{xz - yw, yz + xw, 1 - xx - yy},
	}}
}

// toEulerAnglesXYZ mirrors Matrix3::toEulerAnglesXYZ. The returned angles
// are (X, Y, Z) in radians; the bool is false on the singular branches,
// exactly like G3D (the angles are still written there).
func (a mat3) toEulerAnglesXYZ() (xAngle, yAngle, zAngle float32, ok bool) {
	if a.m[0][2] < 1.0 {
		if a.m[0][2] > -1.0 {
			xAngle = float32(math.Atan2(float64(-a.m[1][2]), float64(a.m[2][2])))
			yAngle = float32(math.Asin(float64(a.m[0][2])))
			zAngle = float32(math.Atan2(float64(-a.m[0][1]), float64(a.m[0][0])))
			return xAngle, yAngle, zAngle, true
		}
		xAngle = -float32(math.Atan2(float64(a.m[1][0]), float64(a.m[1][1])))
		yAngle = -float32(math.Pi / 2)
		zAngle = 0
		return xAngle, yAngle, zAngle, false
	}
	xAngle = float32(math.Atan2(float64(a.m[1][0]), float64(a.m[1][1])))
	yAngle = float32(math.Pi / 2)
	zAngle = 0
	return xAngle, yAngle, zAngle, false
}

func toRadians(deg float32) float32 { return deg * float32(math.Pi) / 180 }

func toDegrees(rad float32) float32 { return rad * 180 / float32(math.Pi) }

func float32FromBits(b uint32) float32 { return math.Float32frombits(b) }
