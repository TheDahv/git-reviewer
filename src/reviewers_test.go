package gitreviewers

import "testing"

func TestMergeStats(t *testing.T) {
	statGroups := []Stats{
		Stats{
			CommitterStat{"a", 1},
			CommitterStat{"b", 2},
		},
		Stats{
			CommitterStat{"a", 3},
			CommitterStat{"c", 4},
		},
	}

	expected := Stats{
		CommitterStat{"a", 4},
		CommitterStat{"b", 2},
		CommitterStat{"c", 4},
	}
	actual := mergeStats(statGroups)

	for i, actualStat := range actual {
		if actualStat != expected[i] {
			t.Errorf("Got\n\t%v\n...expected\n\t%v\n", actualStat, expected[i])
		}
	}
}
