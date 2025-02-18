// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ranges

import "testing"
import "fmt"

func TestSplitRange(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	expected := []string{"kn1", "kn2", "kn3", "kn100"}
	input := "kn[1-3,100]"

	res, _ := r.SplitRange(input)

	es := fmt.Sprintf("%v", expected)
	rs := fmt.Sprintf("%v", res)

	if es != rs {
		t.Fatal("SplitRange returned: ", res, ", expected: ", expected)
	}
}

func TestSplitRangeNoPrefix(t *testing.T) {
	r, _ := NewRange("", 1, 520)

	expected := []string{"1", "2", "3", "100"}
	input := "[1-3,100]"

	res, _ := r.SplitRange(input)

	es := fmt.Sprintf("%v", expected)
	rs := fmt.Sprintf("%v", res)

	if es != rs {
		t.Fatal("SplitRange returned: ", res, ", expected: ", expected)
	}
}

func TestSplitRangePadded(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	expected := []string{"kn008", "kn009", "kn010", "kn011", "kn100"}
	input := "kn[008-011,100]"

	res, _ := r.SplitRange(input)

	es := fmt.Sprintf("%v", expected)
	rs := fmt.Sprintf("%v", res)

	if es != rs {
		t.Fatal("SplitRangePadded returned: ", res, ", expected: ", expected)
	}
}

func TestUnsplitRange(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	expected := "kn[1-5]"
	input := []string{"kn1", "kn2", "kn3", "kn4", "kn5"}

	res, err := r.UnsplitRange(input)
	if err != nil {
		t.Fatal("UnsplitRange returned error: ", err)
	}
	if expected != res {
		t.Fatal("UnsplitRange returned: ", res)
	}

	expected = "kn[1-5,20]"
	input = []string{"kn1", "kn2", "kn3", "kn4", "kn5", "kn20"}

	res, err = r.UnsplitRange(input)
	if err != nil {
		t.Fatal("UnsplitRange returned error: ", err)
	}
	if expected != res {
		t.Fatal("UnsplitRange returned: ", res)
	}

	expected = "kn[1-5,20,44-45]"
	input = []string{"kn44", "kn45", "kn1", "kn2", "kn3", "kn4", "kn5", "kn20"}

	res, err = r.UnsplitRange(input)
	if err != nil {
		t.Fatal("UnsplitRange returned error: ", err)
	}
	if expected != res {
		t.Fatal("UnsplitRange returned: ", res)
	}
}

func TestSplitList(t *testing.T) {
	data := []struct {
		input string
		count int
	}{
		{"foo", 1},
		{"foo,", 1},
		{"foo,bar", 2},
		{"foo,bar[0-1]", 3},
		{"foo,bar[0-1],kn[1,2,3]", 6},
	}

	for _, v := range data {
		res, err := SplitList(v.input)
		if err != nil {
			t.Errorf("expand `%s` -- %v", v.input, err)
		} else if len(res) != v.count {
			t.Errorf("want %d, got %v", v.count, res)
		} else {
			t.Logf("got: %v", res)
		}
	}
}
