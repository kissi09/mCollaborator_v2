package main

import (
	"math"
	"strings"
)

// CVSS v3.1 base score, implemented from the specification's equations.
//
// It exists for one job: a report being imported often gives a vector string
// and no rating word, and a finding has to arrive with a severity. Deriving it
// from the vector is exact where guessing from prose is not.

var cvssAV = map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2}
var cvssAC = map[string]float64{"L": 0.77, "H": 0.44}
var cvssUI = map[string]float64{"N": 0.85, "R": 0.62}
var cvssCIA = map[string]float64{"H": 0.56, "L": 0.22, "N": 0.0}

// Privileges Required is scored differently when the scope changes.
var cvssPRUnchanged = map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}
var cvssPRChanged = map[string]float64{"N": 0.85, "L": 0.68, "H": 0.50}

// cvssBaseScore returns the base score of a v3.x vector string, and false when
// the vector is missing a base metric - a partial vector has no score, and
// inventing one would put a number in the report that nothing backs.
func cvssBaseScore(vector string) (float64, bool) {
	parts := map[string]string{}
	for _, seg := range strings.Split(strings.ToUpper(strings.TrimSpace(vector)), "/") {
		kv := strings.SplitN(seg, ":", 2)
		if len(kv) != 2 {
			continue
		}
		parts[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}

	scope, ok := parts["S"]
	if !ok || (scope != "U" && scope != "C") {
		return 0, false
	}
	prTable := cvssPRUnchanged
	if scope == "C" {
		prTable = cvssPRChanged
	}

	av, ok1 := cvssAV[parts["AV"]]
	ac, ok2 := cvssAC[parts["AC"]]
	pr, ok3 := prTable[parts["PR"]]
	ui, ok4 := cvssUI[parts["UI"]]
	c, ok5 := cvssCIA[parts["C"]]
	i, ok6 := cvssCIA[parts["I"]]
	a, ok7 := cvssCIA[parts["A"]]
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6 && ok7) {
		return 0, false
	}

	iss := 1 - ((1 - c) * (1 - i) * (1 - a))
	var impact float64
	if scope == "U" {
		impact = 6.42 * iss
	} else {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	}
	if impact <= 0 {
		return 0, true
	}

	exploitability := 8.22 * av * ac * pr * ui
	sum := impact + exploitability
	if scope == "C" {
		sum = 1.08 * sum
	}
	return cvssRoundUp(math.Min(sum, 10)), true
}

// cvssRoundUp is the specification's Roundup: the smallest number to one decimal
// place that is not less than the input. Done on an integer to avoid the
// floating-point edge cases the spec calls out.
func cvssRoundUp(x float64) float64 {
	n := int(math.Round(x * 100000))
	if n%10000 == 0 {
		return float64(n) / 100000.0
	}
	return (math.Floor(float64(n)/10000) + 1) / 10.0
}
