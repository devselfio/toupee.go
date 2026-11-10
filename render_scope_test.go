package toupee

import (
	"fmt"
	"testing"
)

// baseRenderingRoster is every corpus case that uses only t-var, t-raw and
// t-comment (after discarding comment bodies), with no transform= attribute
// and no t-var:/t-attr: binding: the scope of issue #11. Each number is the
// line of the case's <!-- test: ... --> marker.
var baseRenderingRoster = map[string][]int{
	"comment.test":        {1, 7, 13, 19, 25, 48},
	"dynamic-path.test":   {1, 9, 17, 25, 33, 41, 49, 97, 105, 113, 121, 129, 137},
	"edge.test":           {1, 6, 11, 17, 23, 29, 37, 45, 53, 79, 87, 95, 103, 111, 127, 139, 147, 168},
	"error.test":          {1, 31, 36, 52, 82, 87, 151, 161},
	"integration.test":    {64, 122, 141},
	"isolation.test":      {53},
	"raw.test":            {1, 9, 17, 25, 30, 36, 44, 52, 57, 62, 86},
	"regression.test":     {4, 24},
	"types-extremes.test": {1, 9, 17, 25, 33},
	"types-render.test":   {1, 9, 17, 25, 33, 41, 49, 57, 65, 73, 81, 89, 97, 105, 113, 121, 129, 137},
	"var.test":            {1, 9, 14, 22, 30, 38, 46, 52, 60, 68, 74, 82, 90, 98, 104, 109, 114, 122, 130, 138, 146, 154, 162},
}

// TestBaseRenderingCorpus renders every roster case with the real Render and
// fails on any that does not pass, whatever the known-failures list says. A
// roster case must exist in the corpus and, once it passes, must not be
// listed as a known failure.
func TestBaseRenderingCorpus(t *testing.T) {
	files := loadCorpus(t)
	cases := map[caseID]preparedCase{}
	for _, f := range files {
		for _, c := range f.cases {
			cases[c.id] = c
		}
	}
	known := loadKnownFailures(t)

	total := 0
	for file, lines := range baseRenderingRoster {
		for _, line := range lines {
			total++
			id := caseID{file, line}
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
	if total != 108 {
		t.Errorf("roster has %d cases, want 108", total)
	}
}
