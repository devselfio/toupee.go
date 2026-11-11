package toupee_test

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

// mixedCompositionRoster is every composition acceptance case for issue #13
// that also needs control flow or let, in the same form as
// coreCompositionRoster. include.test L122 is on neither: it also needs a
// transform on t-let (issue #14), and TestSpec covers it.
var mixedCompositionRoster = map[string][]int{
	"conditional.test":  {106, 115, 124, 134, 400, 409},
	"dynamic-path.test": {57, 65, 73},
	"error.test":        {169},
	"extend.test":       {97, 107},
	"include.test":      {103, 112, 173},
	"integration.test":  {1, 27, 38, 81, 94, 104},
	"isolation.test":    {1, 43},
	"let.test":          {35},
	"nesting.test":      {25, 35, 45, 96, 106, 117, 135},
	"slot.test":         {86, 97},
}

// TestCompositionCoreCorpus renders every core roster case with the real
// Render and fails on any that does not pass, whatever the known-failures
// list says. A roster case must exist in the corpus and must not be listed
// as a known failure.
func TestCompositionCoreCorpus(t *testing.T) {
	checkStrictRoster(t, coreCompositionRoster, 77)
}

// TestCompositionCorpus is issue #13's acceptance: the 77 core and 33 mixed
// cases, which do not overlap, all pass strictly, 110 in all.
func TestCompositionCorpus(t *testing.T) {
	core, mixed := rosterIDs(t, coreCompositionRoster), rosterIDs(t, mixedCompositionRoster)
	if len(core) != 77 || len(mixed) != 33 {
		t.Fatalf("rosters have %d core and %d mixed cases, want 77 and 33", len(core), len(mixed))
	}
	union := map[string][]int{}
	for _, roster := range []map[string][]int{coreCompositionRoster, mixedCompositionRoster} {
		for file, lines := range roster {
			union[file] = append(append([]int(nil), union[file]...), lines...)
		}
	}
	for id := range mixed {
		if core[id] {
			t.Errorf("%s is on both rosters", id)
		}
	}
	checkStrictRoster(t, union, 110)
}

// rosterIDs returns roster's cases, failing on a case listed twice.
func rosterIDs(t *testing.T, roster map[string][]int) map[caseID]bool {
	t.Helper()
	ids := map[caseID]bool{}
	for file, lines := range roster {
		for _, line := range lines {
			id := caseID{file, line}
			if ids[id] {
				t.Errorf("%s is listed twice", id)
			}
			ids[id] = true
		}
	}
	return ids
}

// checkStrictRoster checks every case of roster with checkCase, ignoring the
// known-failures skip policy, and requires want distinct cases, each listed
// once and present once in the corpus.
func checkStrictRoster(t *testing.T, roster map[string][]int, want int) {
	t.Helper()
	files := loadCorpus(t)
	cases := map[caseID]preparedCase{}
	duplicate := map[caseID]bool{}
	for _, f := range files {
		for _, c := range f.cases {
			if _, ok := cases[c.id]; ok {
				duplicate[c.id] = true
			}
			cases[c.id] = c
		}
	}
	known := loadKnownFailures(t)

	ids := rosterIDs(t, roster)
	for id := range ids {
		t.Run(fmt.Sprintf("%s/L%d", id.File, id.Line), func(t *testing.T) {
			c, ok := cases[id]
			if !ok {
				t.Fatalf("%s is not in the corpus", id)
			}
			if duplicate[id] {
				t.Fatalf("%s names more than one corpus case", id)
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
	if len(ids) != want {
		t.Errorf("roster has %d distinct cases, want %d", len(ids), want)
	}
}
