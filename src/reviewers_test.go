package gitreviewers

import (
	"sort"
	"testing"
)

func TestMergeStats(t *testing.T) {
	statGroups := []Stats{
		Stats{
			Stat{"a", 1},
			Stat{"b", 2},
		},
		Stats{
			Stat{"a", 3},
			Stat{"c", 5},
		},
	}

	expected := Stats{
		Stat{"b", 2},
		Stat{"a", 4},
		Stat{"c", 5},
	}
	actual := mergeStats(statGroups)
	sort.Sort(actual)

	for i, actualStat := range actual {
		if actualStat != expected[i] {
			t.Errorf("Got\n\t%v\n...expected\n\t%v\n", actualStat, expected[i])
		}
	}
}
