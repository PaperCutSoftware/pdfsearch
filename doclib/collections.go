// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import "sort"

// IntSliceToSet returns a map whose keys are the elements of `arr`.
func IntSliceToSet(arr []int) map[int]bool {
	set := map[int]bool{}
	for _, x := range arr {
		set[x] = true
	}
	return set
}

// IntSetToSlice returns keys of `set` as a slice.
func IntSetToSlice(set map[int]bool) []int {
	var arr []int
	for x := range set {
		arr = append(arr, x)
	}
	return arr
}

// IntSetUnion returns `a` ∪ `b`.
func IntSetUnion(a, b map[int]bool) map[int]bool {
	u := map[int]bool{}
	for x := range a {
		u[x] = true
	}
	for x := range b {
		u[x] = true
	}
	return u
}

// IntSetIntersection returns `a` ∩ `b`.
func IntSetIntersection(a, b map[int]bool) map[int]bool {
	u := map[int]bool{}
	for x := range a {
		if _, ok := b[x]; ok {
			u[x] = true
		}
	}
	return u
}

// IntSliceUnion returns `a` ∪ `b`.
func IntSliceUnion(a, b []int) []int {
	aa := IntSliceToSet(a)
	bb := IntSliceToSet(b)
	uu := IntSetUnion(aa, bb)
	u := IntSetToSlice(uu)
	sort.Ints(u)
	return u
}

// IntSliceIntersection returns `a` ∩ `b`.
func IntSliceIntersection(a, b []int) []int {
	aa := IntSliceToSet(a)
	bb := IntSliceToSet(b)
	uu := IntSetIntersection(aa, bb)
	u := IntSetToSlice(uu)
	sort.Ints(u)
	return u
}

// IntRange returns slice [i0, i1).
func IntRange(i0, i1 int) []int {
	var arr []int
	for i := i0; i < i1; i++ {
		arr = append(arr, i)
	}
	return arr
}

// IntSliceDifference returns the elements in `a` that aren't in `b`.
func IntSliceDifference(a, b []int) []int {
	mb := map[int]bool{}
	for _, x := range b {
		mb[x] = true
	}
	ab := []int{}
	for _, x := range a {
		if _, ok := mb[x]; !ok {
			ab = append(ab, x)
		}
	}
	sort.Ints(ab)
	return ab
}

// IntSliceDifference returns the elements in `a` that aren't in `b` plus the elements in `b` that
// aren't in `a`.
func IntSliceSymmetricDifference(a, b []int) []int {
	return IntSliceUnion(IntSliceDifference(a, b), IntSliceDifference(b, a))
}

// MinMaxIntSlice returns min and max of `arr`. `valid` is true if `arr` contains values.
func MinMaxIntSlice(arr []int) (min, max int, valid bool) {
	if arr == nil || len(arr) == 0 {
		return
	}
	valid = true
	min = arr[0]
	max = arr[0]
	for _, x := range arr[1:] {
		if x < min {
			min = x
		} else if x > max {
			max = x
		}
	}
	return
}

// StringUniques returns the unique strings in `arr`.
func StringUniques(arr []string) []string {
	set := map[string]bool{}
	var u []string
	for _, s := range arr {
		if _, ok := set[s]; !ok {
			u = append(u, s)
		}
		set[s] = true
	}
	return u
}
