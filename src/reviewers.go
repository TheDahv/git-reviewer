package gitreviewers

import (
	"bytes"
	"container/heap"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	gg "github.com/libgit2/git2go"
)

// ContributionCounter represents a repository and options describing how to
// count changes and attribute them to collaborators to determine experience.
type ContributionCounter struct {
	Repo              *gg.Repository
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

// Less sorts Stats by percentage of "owned" lines per collaborator. This
// implementation bakes in a reverse order, such that higher percentage values
// are sorted first.
func (s Stats) Less(i, j int) bool {
	// This behavior determines the priority order when Stats is Heapified.
	// We want Pop to give us the highest, not lowest, priority.
	return s[i].Percentage > s[j].Percentage
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
	if r.Repo == nil {
		return false, errors.New("repo not initialized")
	}

	var (
		rg      runGuard
		mBranch *gg.Branch
		mCom    *gg.Commit
		hRef    *gg.Reference
		hCom    *gg.Commit
		behind  bool
	)

	defer func() {
		objs := [...]freeable{mBranch, mCom, hRef, hCom}
		for _, obj := range objs {
			obj.Free()
		}
	}()

	rg.maybeRunMany(
		func() {
			mBranch, rg.err = r.Repo.LookupBranch("master", gg.BranchLocal)
		},
		func() {
			mCom, rg.err = r.Repo.LookupCommit(mBranch.Reference.Target())
		},
		func() {
			hRef, rg.err = r.Repo.Head()
		},
		func() {
			hCom, rg.err = r.Repo.LookupCommit(hRef.Target())
		},
		func() {
			behind = hCom.Committer().When.Before(mCom.Committer().When)
		},
	)

	return behind, rg.err
}

// FindFiles returns a list of paths to files that have been changed
// in this branch with respect to "master".
func (r *ContributionCounter) FindFiles() ([]string, error) {
	var (
		rg      runGuard
		paths   []string
		mBranch *gg.Branch
		hRef    *gg.Reference
		hCom    *gg.Commit
		mCom    *gg.Commit
		mTree   *gg.Tree
		hTree   *gg.Tree
		opts    gg.DiffOptions
		diff    *gg.Diff
	)

	if r.Repo == nil {
		return paths, errors.New("repo not initialized")
	}

	defer func() {
		objs := [...]freeable{mBranch, hRef, hCom, mCom, mTree, hTree}

		for _, obj := range objs {
			if obj != nil {
				obj.Free()
			}
		}

		if err := diff.Free(); err != nil && r.Verbose {
			fmt.Printf("Issue cleaning up diff: '%s'\n", err)
		}
	}()

	rg.maybeRunMany(
		func() {
			mBranch, rg.err = r.Repo.LookupBranch("master", gg.BranchLocal)
			rg.msg = "issue opening master branch"
		},
		func() {
			mCom, rg.err = r.Repo.LookupCommit(mBranch.Reference.Target())
			rg.msg = "issue opening commit at master"
		},
		func() {
			hRef, rg.err = r.Repo.Head()
			rg.msg = "issue opening repo at HEAD"
		},
		func() {
			hCom, rg.err = r.Repo.LookupCommit(hRef.Target())
			rg.msg = "issue opening commit at HEAD"
		},
		func() {
			mTree, rg.err = mCom.Tree()
			rg.msg = "issue opening tree at master"
		},
		func() {
			hTree, rg.err = hCom.Tree()
			rg.msg = "issue opening tree at HEAD"
		},
		func() {
			opts, rg.err = gg.DefaultDiffOptions()
			rg.msg = "issue creating diff options"
		},
		func() {
			diff, rg.err = r.Repo.DiffTreeToTree(mTree, hTree, &opts)
			rg.msg = "issue finding diff"
		},
		func() {
			diff.ForEach(func(file gg.DiffDelta, progress float64) (
				gg.DiffForEachHunkCallback, error) {

				// Only include path if it passes all filters
				path := file.OldFile.Path
				if considerExt(path, r) && considerPath(path, r) {
					paths = append(paths, path)
				}
				return nil, nil
			}, gg.DiffDetailFiles)
		},
	)

	if rg.err != nil && rg.msg != "" && r.Verbose {
		fmt.Printf("Error finding diff files: '%s'\n", rg.msg)
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
		rg              runGuard
		since           time.Time
		final           Stats
		opts            gg.BlameOptions
		totalLines      uint16
		linesByCommiter = make(map[string]float64)
	)

	mm, _ := readMailmap()

	if len(r.Since) > 0 {
		var err error
		since, err = time.Parse("2006-01-02", r.Since)
		if err != nil {
			if r.Verbose {
				fmt.Println("Unable to parse 'since'")
			}
			return "", err
		}
	} else {
		// Calculate 6 months ago from today's date and set the 'since' argument
		since = time.Now().AddDate(0, -6, 0)
	}

	rg.maybeRun(func() {
		opts, rg.err = gg.DefaultBlameOptions()
		rg.msg = "Issue creating blame options"
	})

	// Obtain a blame for each file in the diff and count up experience by lines,
	// discarding hunks from comits older than the 'since' threshold.
	rg.maybeRun(func() {
		for _, p := range paths {
			b, err := r.Repo.BlameFile(p, &opts)
			if err != nil {
				continue
			}

			cnt := b.HunkCount()
			for i := 0; i < cnt; i++ {
				h, err := b.HunkByIndex(i)

				// Skip for errors or for hunks that are older than our window of
				// consideration.
				if err != nil || h.OrigSignature.When.Before(since) {
					continue
				}

				k := reviewerKey(h.OrigSignature, mm)
				linesByCommiter[k] += float64(h.LinesInHunk)
				totalLines += h.LinesInHunk
			}

			b.Free()
		}
	})

	// Calculate percentage of "ownership" by percent of all lines touched
	for commiter, lines := range linesByCommiter {
		// Calculate percent of lines touched in-place
		lines := lines
		linesByCommiter[commiter] = lines / float64(totalLines)
	}

	final = make(Stats, len(linesByCommiter))
	idx := 0
	for c, p := range linesByCommiter {
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

// reviewerKey returns the formatted name and email for a collaborator on the
// signature of a git object. It uses the provided mailmap to determine the
// normalized name and email, or uses the original name and email on the
// signature if not contained in the mailmap.
func reviewerKey(sig *gg.Signature, mm mailmap) string {
	var (
		name, email string
		ok          bool
	)

	if name, ok = mm[sig.Name]; !ok {
		name = sig.Name
	}

	if email, ok = mm[sig.Email]; !ok {
		email = sig.Email
	}

	return fmt.Sprintf("%s <%s>", name, email)
}

// chooseTopN consumes the greatest 'n' Stat objects from a Stats list.
func chooseTopN(n int, s Stats) Stats {
	top := make(Stats, n)
	ti := 0

	heap.Init(&s)
	for i := 0; i < n; i++ {
		val := heap.Pop(&s)
		if val != nil {
			top[ti] = val.(*Stat)
			ti++
		}
	}

	return top
}
