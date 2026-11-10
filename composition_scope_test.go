package toupee

import (
	"fmt"
	"testing"
)

// coreCompositionRoster is every composition acceptance case for issue #13
// that needs no control flow, let or transform: includes, extends, slots,
// definitions, t-attr and t-var:/t-attr: bindings, and the dynamic data
// paths that reach them. Each number is the line of the case's
// <!-- test: ... --> marker.
var coreCompositionRoster = map[string][]int{
	"bind.test":         {1, 9, 15, 23, 32, 41, 49, 59, 67, 75, 83},
	"comment.test":      {39},
	"dynamic-path.test": {1, 9, 17, 25, 33, 41, 49, 81, 97, 105, 113, 121, 129, 137},
	"edge.test":         {61, 184},
	"error.test":        {41, 47, 62, 69, 97, 112, 118, 124, 133, 142},
	"extend.test":       {1, 13, 26, 39, 52, 58, 70, 83, 118},
	"include.test":      {1, 10, 19, 30, 42, 53, 58, 68, 80, 90, 131, 140, 149, 160},
	"integration.test":  {16, 70, 130},
	"isolation.test":    {11, 21, 31, 53},
	"regression.test":   {35},
	"slot.test":         {1, 11, 22, 32, 42, 53, 63, 76},
}

// TestCompositionCoreCorpus renders every roster case with the real Render
// and fails on any that does not pass, whatever the known-failures list
// says. A roster case must exist in the corpus and must not be listed as a
// known failure.
func TestCompositionCoreCorpus(t *testing.T) {
	checkStrictRoster(t, coreCompositionRoster, 77)
}

// checkStrictRoster checks every case of roster with checkCase, ignoring the
// known-failures skip policy, and requires want distinct cases.
func checkStrictRoster(t *testing.T, roster map[string][]int, want int) {
	t.Helper()
	files := loadCorpus(t)
	cases := map[caseID]preparedCase{}
	for _, f := range files {
		for _, c := range f.cases {
			cases[c.id] = c
		}
	}
	known := loadKnownFailures(t)

	seen := map[caseID]bool{}
	for file, lines := range roster {
		for _, line := range lines {
			id := caseID{file, line}
			seen[id] = true
			t.Run(fmt.Sprintf("%s/L%d", file, line), func(t *testing.T) {
				c, ok := cases[id]
				if !ok {
					t.Fatalf("%s is not in the corpus", id)
				}
				var err error
				func() {
					defer func() {
						if r := recover(); r != nil {
							err = fmt.Errorf("%s: Render panicked: %v", id, r)
						}
					}()
					err = checkCase(c)
				}()
				if err != nil {
					t.Fatal(err)
				}
				if known[id] {
					t.Errorf("%s passes: delete its entry from %s", id, knownFailuresPath)
				}
			})
		}
	}
	if len(seen) != want {
		t.Errorf("roster has %d distinct cases, want %d", len(seen), want)
	}
}
