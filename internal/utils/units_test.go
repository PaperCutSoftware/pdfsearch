// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package utils

import (
	"math"
	"testing"
)

// TesUnits checks that f(g(x)) == x where g and f are inverses for all the functions in units.
func TesUnits(t *testing.T) {
	for y := -100.0; y <= 100.0; y += 1.0 {
		x := math.Exp(y)
		tol := math.Max(x/100.0, 1e10)
		unitsMmPtMm(t, x, tol)
		unitsMmPtMm(t, -x, tol)
		unitsPtMmPt(t, x, tol)
		unitsPtMmPt(t, -x, tol)
	}
}

// unitsMmPtMm checks that PointToMM(MMToPoint(x)) == x.
func unitsMmPtMm(t *testing.T, x, tol float64) {
	pt := MMToPoint(x)
	mm := PointToMM(pt)
	if math.Abs(mm-x) > tol {
		t.Fatalf("mm=%g != x=%g (pt=%g) mm-x=%g", mm, x, pt, mm-x)
	}
}

// unitsPtMmPt checks that  MMToPoint(PointToMM(x)) == x.
func unitsPtMmPt(t *testing.T, x, tol float64) {
	mm := PointToMM(x)
	pt := MMToPoint(mm)

	if math.Abs(pt-x) > tol {
		t.Fatalf("pt=%g != x=%g (mm=%g) pt-x=%g", pt, x, mm, pt-x)
	}
}
