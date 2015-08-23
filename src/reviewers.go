package gitreviewers

import (
	"fmt"
	"sort"
)

// Stat contains contributor name and commit count summary. It is
// well-suited for capturing information returned from git shortlog.
type Stat struct {
	Reviewer string
	Count    int
}

// String shows Stat information in a format suitable for shell reporting.
func (cs *Stat) String() string {
	return fmt.Sprintf("  %d\t%s", cs.Count, cs.Reviewer)
}

// Stats is a convenience type that lets us implement the sortable interface.
type Stats []Stat

// Len returns the number of Stat objects.
func (s Stats) Len() int {
	return len(s)
}

// Less sorts Stats by the commit count in each Stat.
func (s Stats) Less(i, j int) bool {
	return s[i].Count < s[j].Count
}

// Swap implements in-place sorting.
func (s Stats) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// BranchBehind is not yet implemented. Determines if the current branch
// behind master and requires that it be "merged up".
func BranchBehind() bool {
	return false
}

// FindFiles returns a list of paths to files that have been changed
// in this branch.
func FindFiles() ([]string, error) {
	return changedFiles()
}

// mergeStats takes a list of stats and groups them by Reviewer, summing
// total commit count for each. A new Stats with all data merged is returned.
func mergeStats(stats []Stats) Stats {
	var final Stats
	reviewers := make(map[string]int)

	for _, group := range stats {
		for _, stat := range group {
			reviewers[stat.Reviewer] += stat.Count
		}
	}

	for reviewer, count := range reviewers {
		final = append(final, Stat{reviewer, count})
	}

	return final
}

// FindReviewers returns up to 3 of the top reviewers information as determined
// by cumulative commit count across all files in `paths`.
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
