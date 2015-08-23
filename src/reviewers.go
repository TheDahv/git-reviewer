package gitreviewers

import (
	"fmt"
	"sort"
)

type CommitterStat struct {
	Reviewer string
	Count    int
}

func (cs *CommitterStat) String() string {
	return fmt.Sprintf("  %d\t%s", cs.Count, cs.Reviewer)
}

// Sortable Stats
type Stats []CommitterStat

func (s Stats) Len() int {
	return len(s)
}

func (s Stats) Less(i, j int) bool {
	return s[i].Count < s[j].Count
}

func (s Stats) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func BranchBehind() bool {
	return false
}

func FindFiles() ([]string, error) {
	return changedFiles()
}

func mergeStats(stats []Stats) Stats {
	var final Stats
	reviewers := make(map[string]int)

	for _, group := range stats {
		for _, stat := range group {
			reviewers[stat.Reviewer] += stat.Count
		}
	}

	for reviewer, count := range reviewers {
		final = append(final, CommitterStat{reviewer, count})
	}

	return final
}

func FindReviewers(paths []string) ([]string, error) {
	var (
		finalStats Stats
		results    []string
		statGroups []Stats
	)

	for _, path := range paths {
		stats, err := committerCounts(path)
		if err != nil {
			// This path must not exist upstream, so lets skip it
			fmt.Println("Skipping " + path)
			continue
		}
		statGroups = append(statGroups, stats)
	}

	// Turn map back into Stats so we can sort
	finalStats = mergeStats(statGroups)
	sort.Sort(sort.Reverse(finalStats))

	// Grab top 3 reviewers and return string lines
	maxStats := 3
	if l := len(finalStats); l < maxStats {
		maxStats = l
	}
	for _, stat := range finalStats[:maxStats] {
		results = append(results, stat.String())
	}

	return results, nil
}
