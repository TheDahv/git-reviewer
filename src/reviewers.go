package gitreviewers

import (
	"bytes"
	"container/heap"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

// ContributionCounter represents a repository and options describing how to
// count changes and attribute them to collaborators to determine experience.
type ContributionCounter struct {
	Repo              *gogit.Repository
	ShowFiles         bool
	Verbose           bool
	Since             string
	IgnoredExtensions []string
	OnlyExtensions    []string
	IgnoredPaths      []string
	OnlyPaths         []string
}

// Stat contains information about a collaborator and the total "experience"
// in a branch as determined by the percentage of lines owned out of the total
// number of lines of code in a changed file.
type Stat struct {
	Reviewer   string
	Percentage float64
}

// String shows Stat information in a format suitable for shell reporting.
func (cs *Stat) String() string {
	return fmt.Sprintf("  %.2f%%\t%s", cs.Percentage*100.0, cs.Reviewer)
}

// Stats is a collection of all the collaboration statistics obtained across
// changes in a repository. By defining our own slice type, we are able to
// add methods to implement the Heap interface, which we use to determine
// collaborators with the most experience without sorting the entire list.
type Stats []*Stat

// Len returns the number of Stat objects.
func (s Stats) Len() int {
	return len(s)
}

// Less sorts Stats by percentage of "owned" lines per collaborator.
func (s Stats) Less(i, j int) bool {
	// This behavior determines the priority order when Stats is Heapified.
	// We want Pop to give us the highest, not lowest, priority.
	return s[i].Percentage < s[j].Percentage
}

// Swap moves elements around to their proper location in the heap
func (s Stats) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Pop removes the largest Stat from the queue and returns it.
func (s *Stats) Pop() interface{} {
	var stat *Stat
	n := len(*s)
	if n == 0 {
		return nil
	}

	stat = (*s)[n-1]
	*s = (*s)[0 : n-1]

	return stat
}

// Push adds a stat into the priority queue
func (s *Stats) Push(val interface{}) {
	*s = append(*s, val.(*Stat))
}

// freeable types allow clients to free memory when they are finished with them
type freeable interface {
	Free()
}

// defaultIgnoreExt are filetypes extensions that are more often machine-edited
// and are less likely to reflect actual experience on a project
var defaultIgnoreExt = []string{
	"svg",
	"json",
	"nock",
	"xml",
}

// BranchBehind determines if the current branch is "behind"
// by comparing the current branch HEAD reference to that of the local ref of
// the master branch.
func (r *ContributionCounter) BranchBehind() (bool, error) {
	var (
		rg     runGuard
		m      *plumbing.Reference
		mObj   *object.Commit
		h      *plumbing.Reference
		hObj   *object.Commit
		behind bool
	)

	rg.maybeRunMany(
		func() {
			m, rg.err = r.Repo.Reference(plumbing.Master, true)
			rg.msg = "issue opening master reference"
		},
		func() {
			h, rg.err = r.Repo.Reference(plumbing.HEAD, true)
			rg.msg = "issue opening HEAD reference"
		},
		func() {
			mObj, rg.err = r.Repo.CommitObject(m.Hash())
			rg.msg = "issue opening master commit"
		},
		func() {
			hObj, rg.err = r.Repo.CommitObject(h.Hash())
			rg.msg = "issue opening HEAD commit"
		},
		func() {
			behind = hObj.Committer.When.Before(mObj.Committer.When)
			rg.msg = "issue comparing commit dates"
		},
	)

	if rg.err != nil && rg.msg != "" && r.Verbose {
		fmt.Printf("Error comparing branches: '%s'\n", rg.msg)
	}

	return behind, rg.err
}

// FindFiles returns a list of paths to files that have been changed
// in this branch with respect to "master".
func (r *ContributionCounter) FindFiles() ([]string, error) {
	var (
		rg      runGuard
		m       *plumbing.Reference
		mc      *object.Commit
		h       *plumbing.Reference
		hc      *object.Commit
		mt      *object.Tree
		ht      *object.Tree
		changes object.Changes
		paths   []string
	)

	set := make(map[string]bool)

	rg.maybeRunMany(
		func() {
			m, rg.err = r.Repo.Reference(plumbing.Master, true)
			rg.msg = "issue opening master ref"
		},
		func() {
			mc, rg.err = r.Repo.CommitObject(m.Hash())
			rg.msg = "issue opening master commit"
		},
		func() {
			mt, rg.err = mc.Tree()
			rg.msg = "issue opening tree at master"
		},
		func() {
			h, rg.err = r.Repo.Reference(plumbing.HEAD, true)
			rg.msg = "issue opening HEAD ref"
		},
		func() {
			hc, rg.err = r.Repo.CommitObject(h.Hash())
			rg.msg = "issue opening HEAD commit"
		},
		func() {
			ht, rg.err = hc.Tree()
			rg.msg = "issue opening tree at HEAD"
		},
		func() {
			changes, rg.err = object.DiffTree(mt, ht)
			rg.msg = "issue diffing master and head trees"
		},
		func() {
			for _, ch := range changes {
				n := ch.To.Name
				if considerExt(n, r) && considerPath(n, r) {
					set[n] = true
				}
			}
		},
	)

	if rg.err != nil && rg.msg != "" && r.Verbose {
		fmt.Printf("Error finding diff files: '%s'\n", rg.msg)
	}

	for path := range set {
		paths = append(paths, path)
	}

	return paths, rg.err
}

// considerExt determines whether a path should be used to calculate the final
// collaborators score based on the inclusion or absence of its extension in the
// list of paths to exlusively include or exclude, respectively.
func considerExt(path string, opts *ContributionCounter) bool {
	ignExt := []string{}
	ignExt = append(ignExt, defaultIgnoreExt...)
	ignExt = append(ignExt, opts.IgnoredExtensions...)

	lAllow, lIgnore := len(opts.OnlyExtensions), len(ignExt)

	if lAllow == 0 && lIgnore == 0 {
		return true
	}

	if lAllow > 0 {
		for _, ext := range opts.OnlyExtensions {
			if strings.HasSuffix(path, ext) {
				return true
			}
		}
	} else if lIgnore > 0 {
		passes := true
		for _, ext := range ignExt {
			passes = passes && !strings.HasSuffix(path, ext)
		}

		return passes
	}

	return false
}

// considerPath determines whether a path should be used to calculate the final
// collaborators score based on its inclusion or absence in the list of paths to
// exlusively include or exclude, respectively.
func considerPath(path string, opts *ContributionCounter) bool {
	lAllow, lIgnore := len(opts.OnlyPaths), len(opts.IgnoredPaths)
	pLen := len(path)

	if lAllow == 0 && lIgnore == 0 {
		return true
	}

	if lAllow > 0 {
		for _, prefix := range opts.OnlyPaths {
			if len(strings.TrimPrefix(path, prefix)) < pLen {
				return true
			}
		}
	} else if lIgnore > 0 {
		passes := true
		for _, prefix := range opts.IgnoredPaths {
			passes = passes && len(strings.TrimPrefix(path, prefix)) == pLen
		}

		return passes
	}
	return false
}

// FindReviewers returns up to 3 of the top reviewers information as determined
// by percentage of owned lines of all lines in changed file.
func (r *ContributionCounter) FindReviewers(paths []string) (string, error) {
	var (
		rg               runGuard
		final            Stats
		totalLines       uint16
		linesByCommitter = make(map[string]float64)
		m                *plumbing.Reference
		mc               *object.Commit
	)

	mm, _ := readMailmap()

	// Get the master commit so we can determine what the experience was *before*
	// the author got to the file.
	rg.maybeRunMany(
		func() {
			m, rg.err = r.Repo.Reference(plumbing.Master, true)
			rg.msg = "unable to find ref for master"
		},
		func() {
			mc, rg.err = r.Repo.CommitObject(m.Hash())
			rg.msg = "unable to find commit for master"
		},
		func() {
			for _, p := range paths {
				b, err := gogit.Blame(mc, p)
				if err != nil {
					// TODO Swallowing blame errors for now. Handle this somehow
					continue
				}

				for _, l := range b.Lines {
					k := reviewerKey(l.Author, mm)
					linesByCommitter[k] += 1
					totalLines += 1
				}
			}
		},
	)

	for author, lines := range linesByCommitter {
		// Calculate percent of lines touched in-place
		lines := lines
		linesByCommitter[author] = lines / float64(totalLines)
	}

	final = make(Stats, len(linesByCommitter))
	idx := 0
	for c, p := range linesByCommitter {
		final[idx] = &Stat{c, p}
		idx++
	}

	maxStats := 3
	if l := len(final); l < maxStats {
		maxStats = l
	}
	topN := chooseTopN(maxStats, final)

	var buffer bytes.Buffer
	tw := tabwriter.NewWriter(&buffer, 0, 8, 1, '\t', 0)

	fmt.Fprintln(tw, "Reviewer\tExperience")
	fmt.Fprintln(tw, "--------\t----------")

	for i := range topN {
		fmt.Fprintf(tw, "%s\t%.2f%%\n", topN[i].Reviewer, topN[i].Percentage*100.0)
	}
	tw.Flush()

	return buffer.String(), nil
}

// reviewerKey resolves an author email to its canonical in the mailmap
func reviewerKey(email string, mm mailmap) string {
	if e, ok := mm[email]; ok {
		email = e
	}

	return email
}

// chooseTopN consumes the greatest 'n' Stat objects from a Stats list.
func chooseTopN(n int, s Stats) Stats {
	var top Stats
	heap.Init(&top)

	for _, stat := range s {
		stat := stat

		if top.Len() < n || stat.Percentage > top[0].Percentage {
			// Replace the largest item in the heap with this one
			// This way our heap never grows larger than it needs to be
			if top.Len() == n {
				heap.Pop(&top)
			}

			heap.Push(&top, stat)
		}
	}
	sort.Sort(sort.Reverse(top))

	return top
}
