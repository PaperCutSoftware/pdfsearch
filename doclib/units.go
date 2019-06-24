// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

const (
	Inch2MM    = 25.4
	Inch2Point = 72.0
	MM2Point   = Inch2Point / Inch2MM
	Point2MM   = Inch2MM / Inch2Point
)

func MMToPoint(x float64) float64 {
	return x * MM2Point
}

func PointToMM(x float64) float64 {
	return x * Point2MM
}
