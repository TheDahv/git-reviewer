package gitreviewers

import (
	"fmt"
	"sort"
	"strings"
)

// Stat contains contributor name and commit count summary. It is
// well-suited for capturing information returned from git shortlog.
type Stat struct {
	Reviewer string
	Count    int
}

// Carries information for the completion and possible error of
// a stat finder process.
type statResp struct {
	path string
	err  error
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

// Reviewer manages the operations and sequencing of the branch reviewer
type Reviewer struct {
	ShowFiles         bool
	Verbose           bool
	Since             string
	IgnoredExtensions []string
	OnlyExtensions    []string
	IgnoredPaths      []string
}

// BranchBehind is not yet implemented. Determines if the current branch
// behind master and requires that it be "merged up".
func (r *Reviewer) BranchBehind() (bool, error) {
	var master, current string
	var err error

	if master, err = commitTimeStamp("master"); err != nil {
		return false, err
	}

	if current, err = commitTimeStamp("HEAD"); err != nil {
		return false, err
	}

	return current < master, nil
}

// FindFiles returns a list of paths to files that have been changed
// in this branch with respect to `master`.
func (r *Reviewer) FindFiles() ([]string, error) {
	var lines []string
	out, err := run("git diff master HEAD --name-only")

	if err != nil {
		return lines, err
	}

	for _, line := range strings.Split(out, "\n") {
		l := strings.Trim(line, " ")

		var passExtCheck bool

		// OnlyExtensions should take priority over IgnoredExtensions if both
		// happen to be defined
		if len(r.OnlyExtensions) > 0 {
			passExtCheck = false
			for _, ext := range r.OnlyExtensions {
				passExtCheck = passExtCheck || strings.HasSuffix(line, ext)
			}
		} else if len(r.IgnoredExtensions) > 0 {
			passExtCheck = true
			for _, ext := range r.IgnoredExtensions {
				passExtCheck = passExtCheck && !strings.HasSuffix(line, ext)
			}
		}

		passPathCheck := true
		lLen := len(line)
		if len(r.IgnoredPaths) > 0 {
			for _, prefix := range r.IgnoredPaths {
				passPathCheck = passPathCheck &&
					len(strings.TrimPrefix(line, prefix)) == lLen
			}
		}

		if len(l) > 0 && passExtCheck && passPathCheck {
			lines = append(lines, l)
		}
	}

	return lines, err
}

// FindReviewers returns up to 3 of the top reviewers information as determined
// by cumulative commit count across all files in `paths`.
func (r *Reviewer) FindReviewers(paths []string) ([]string, error) {
	var (
		finalStats Stats
		results    []string
	)

	merged := make(map[string]int)
	statCh := make(chan Stat)
	opCh := make(chan statResp)

	for _, path := range paths {
		go func(path string) {
			committerCounts(path, r.Since, statCh, opCh)
		}(path)
	}

	// Loop and merge stats into single map until all ops are done
	for i := 0; i < len(paths); {
		select {
		case stat := <-statCh:
			merged[stat.Reviewer] += stat.Count
		case signal := <-opCh:
			if signal.err != nil && r.Verbose {
				// This path must not exist upstream, so lets skip it
				fmt.Println("Skipping " + signal.path)
			}

			i++
		}
	}

	close(statCh)
	close(opCh)

	// Convert merged into Stats[]
	for reviewer, count := range merged {
		finalStats = append(finalStats, Stat{reviewer, count})
	}

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
