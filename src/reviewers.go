package gitreviewers

import (
	"bytes"
	"container/heap"
	"errors"
	"fmt"
	"strings"
	"time"

	gg "github.com/libgit2/git2go"
)

// Stat contains contributor name and commit count summary. It is
// well-suited for capturing information returned from git shortlog.
type Stat struct {
	Reviewer   string
	Percentage float64
}

// Carries information for the completion and possible error of
// a stat finder process.
type statResp struct {
	path string
	err  error
}

// String shows Stat information in a format suitable for shell reporting.
func (cs *Stat) String() string {
	return fmt.Sprintf("  %.2f%%\t%s", cs.Percentage*100.0, cs.Reviewer)
}

// Stats is a convenience type that lets us implement the heap interface.
type Stats []*Stat

// Len returns the number of Stat objects.
func (s Stats) Len() int {
	return len(s)
}

// Less sorts Stats by the commit count in each Stat.
func (s Stats) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we
	// use greater than here.
	//return s[i].Count > s[j].Count
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

// Reviewer manages the operations and sequencing of the branch reviewer
type Reviewer struct {
	Repo              *gg.Repository
	ShowFiles         bool
	Verbose           bool
	Since             string
	IgnoredExtensions []string
	OnlyExtensions    []string
	IgnoredPaths      []string
	OnlyPaths         []string
}

// freeable makes it easier for us to deal with objects of all types
// that require being freed at the end of a function.
type freeable interface {
	Free()
}

// defaultIgnoreExt represent filetypes that are more often
// machine-edited and are less likely to reflect actual experience
// on a project
var defaultIgnoreExt = []string{
	"svg",
	"json",
	"nock",
	"xml",
}

// BranchBehind is not yet implemented. Determines if the current branch
// behind master and requires that it be "merged up".
func (r *Reviewer) BranchBehind() (bool, error) {
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
		objs := [...]freeable{
			mBranch,
			mCom,
			hRef,
			hCom,
		}
		for _, obj := range objs {
			obj.Free()
		}
	}()

	rg.maybeRun(func() {
		var err error
		if mBranch, err = r.Repo.LookupBranch("master", gg.BranchLocal); err != nil {
			rg.err = err
			rg.msg = "Issue opening master branch"
		}
	})
	rg.maybeRun(func() {
		var err error
		if mCom, err = r.Repo.LookupCommit(mBranch.Reference.Target()); err != nil {
			rg.err = err
			rg.msg = "Issue opening master commit"
		}
	})
	rg.maybeRun(func() {
		var err error
		if hRef, err = r.Repo.Head(); err != nil {
			rg.err = err
			rg.msg = "Issue opening HEAD reference"
		}
	})
	rg.maybeRun(func() {
		var err error
		if hCom, err = r.Repo.LookupCommit(hRef.Target()); err != nil {
			rg.err = err
			rg.msg = "Issue opening HEAD commit"
		}
	})
	rg.maybeRun(func() {
		behind = hCom.Committer().When.Before(mCom.Committer().When)
	})

	return behind, rg.err
}

// FindFiles returns a list of paths to files that have been changed
// in this branch with respect to `master`.
func (r *Reviewer) FindFiles() ([]string, error) {
	var (
		rg      runGuard
		lines   []string
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
		return lines, errors.New("repo not initialized")
	}

	defer func() {
		objs := [...]freeable{
			mBranch,
			hRef,
			hCom,
			mCom,
			mTree,
			hTree,
		}

		for _, obj := range objs {
			if obj != nil {
				obj.Free()
			}
		}

		if err := diff.Free(); err != nil && r.Verbose {
			fmt.Printf("Issue cleaning up diff: '%s'\n", err)
		}
	}()

	rg.maybeRun(func() {
		var err error
		if mBranch, err = r.Repo.LookupBranch("master", gg.BranchLocal); err != nil {
			rg.err = err
			rg.msg = "issue opening master branch"
		}
	})

	rg.maybeRun(func() {
		var err error
		if mCom, err = r.Repo.LookupCommit(mBranch.Reference.Target()); err != nil {
			rg.err = err
			rg.msg = "issue opening commit at master"
		}
	})

	rg.maybeRun(func() {
		var err error
		if hRef, err = r.Repo.Head(); err != nil {
			rg.err = err
			rg.msg = "issue opening repo at HEAD"
		}
	})

	rg.maybeRun(func() {
		var err error
		if hCom, err = r.Repo.LookupCommit(hRef.Target()); err != nil {
			rg.err = err
			rg.msg = "issue opening commit at HEAD"
		}
	})

	rg.maybeRun(func() {
		var err error
		if mTree, err = mCom.Tree(); err != nil {
			rg.err = err
			rg.msg = "issue opening tree at master"
		}
	})

	rg.maybeRun(func() {
		var err error
		if hTree, err = hCom.Tree(); err != nil {
			rg.err = err
			rg.msg = "issue opening tree at HEAD"
		}
	})

	rg.maybeRun(func() {
		var err error
		if opts, err = gg.DefaultDiffOptions(); err != nil {
			rg.err = err
			rg.msg = "issue creating diff options"
		}
	})

	rg.maybeRun(func() {
		var err error
		if diff, err = r.Repo.DiffTreeToTree(mTree, hTree, &opts); err != nil {
			rg.err = err
			rg.msg = "issue finding diff"
		}
	})

	rg.maybeRun(func() {
		diff.ForEach(func(file gg.DiffDelta, progress float64) (
			gg.DiffForEachHunkCallback, error) {

			lines = append(lines, file.OldFile.Path)
			return nil, nil
		}, gg.DiffDetailFiles)
	})

	if rg.err != nil && rg.msg != "" && r.Verbose {
		fmt.Printf("Error finding diff files: '%s'\n", rg.msg)
	}

	return lines, rg.err
}

func considerExt(path string, opts *Reviewer) bool {
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

func considerPath(path string, opts *Reviewer) bool {
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
// by cumulative commit count across all files in `paths`.
func (r *Reviewer) FindReviewers(paths []string) (string, error) {
	var (
		rg                 runGuard
		rw                 *gg.RevWalk
		since              time.Time
		final              Stats
		opts               gg.BlameOptions
		totalLines         uint16
		linesByCommiter    = make(map[string]uint16)
		commitersByPercent = make(map[string]float64)
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

	// Cleanup
	defer func() {
		objs := [...]freeable{
			rw,
		}
		for _, obj := range objs {
			if obj != nil {
				obj.Free()
			}
		}
	}()

	rg.maybeRun(func() {
		var err error

		if opts, err = gg.DefaultBlameOptions(); err != nil {
			rg.err = err
			rg.msg = "Issue creating blame options"
		}
	})

	// Iterate through commits in the review period
	rg.maybeRun(func() {
		var err error
		if rw, err = r.Repo.Walk(); err != nil {
			rg.err = err
			rg.msg = "Issue opening revwalk"
		}

		rw.Sorting(gg.SortTime | gg.SortTopological)
	})

	rg.maybeRun(func() {
		var err error
		// TODO push master, not HEAD
		if err = rw.PushHead(); err != nil {
			rg.err = err
			rg.msg = "Issue pushing HEAD onto revwalk"
		}
	})

	// Now that we know the oldest commit, obtain a blame for each file in the
	// diff and count up experience by lines
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
				linesByCommiter[k] += h.LinesInHunk
				totalLines += h.LinesInHunk
			}

			b.Free()
		}
	})

	// Calculate percentage of "ownership" by percent of all lines touched
	for commiter, lines := range linesByCommiter {
		commitersByPercent[commiter] = float64(lines) / float64(totalLines)
	}

	final = make(Stats, len(commitersByPercent))
	idx := 0
	for c, p := range commitersByPercent {
		final[idx] = &Stat{c, p}
		idx++
	}

	maxStats := 3
	if l := len(final); l < maxStats {
		maxStats = l
	}
	topN := chooseTopN(maxStats, final)

	var buffer bytes.Buffer
	for i := range topN {
		buffer.WriteString(topN[i].String())
		buffer.WriteString("\n")
	}

	return buffer.String(), nil
}

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
