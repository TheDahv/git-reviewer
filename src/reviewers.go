package gitreviewers

import (
	"bytes"
	"container/heap"
	"fmt"
	"strings"
	"sync"

	gg "github.com/libgit2/git2go"
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
	return s[i].Count > s[j].Count
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
	ShowFiles         bool
	Verbose           bool
	Since             string
	IgnoredExtensions []string
	OnlyExtensions    []string
	IgnoredPaths      []string
	OnlyPaths         []string
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

// freeable makes it easier for us to deal with objects of all types
// that require being freed at the end of a function.
type freeable interface {
	Free()
}

// FindFiles returns a list of paths to files that have been changed
// in this branch with respect to `master`.
func (r *Reviewer) FindFiles(repoPath string) ([]string, error) {
	var (
		rg      runGuard
		lines   []string
		repo    *gg.Repository
		mBranch *gg.Branch
		hRef    *gg.Reference
		hCom    *gg.Commit
		mCom    *gg.Commit
		mTree   *gg.Tree
		hTree   *gg.Tree
		opts    gg.DiffOptions
		diff    *gg.Diff
	)

	defer func() {
		objs := [...]freeable{
			repo,
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
		if repo, err = gg.OpenRepository(repoPath); err != nil {
			rg.err = err
			rg.msg = "issue opening repository"
		}
	})

	rg.maybeRun(func() {
		var err error
		if mBranch, err = repo.LookupBranch("master", gg.BranchLocal); err != nil {
			rg.err = err
			rg.msg = "issue opening master branch"
		}
	})

	rg.maybeRun(func() {
		var err error
		if mCom, err = repo.LookupCommit(mBranch.Reference.Target()); err != nil {
			rg.err = err
			rg.msg = "issue opening commit at master"
		}
	})

	rg.maybeRun(func() {
		var err error
		if hRef, err = repo.Head(); err != nil {
			rg.err = err
			rg.msg = "issue opening repo at HEAD"
		}
	})

	rg.maybeRun(func() {
		var err error
		if hCom, err = repo.LookupCommit(hRef.Target()); err != nil {
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
		if diff, err = repo.DiffTreeToTree(mTree, hTree, &opts); err != nil {
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
	cs := make([]chan *Stat, len(paths))
	for i, path := range paths {
		cs[i] = committerCounts(path, r.Since)
	}

	data := mergeChans(cs...)

	set := make(map[string]*Stat)
	for stat := range data {
		if len(stat.Reviewer) > 0 {
			if s, ok := set[stat.Reviewer]; ok {
				s.Count += stat.Count
			} else {
				set[stat.Reviewer] = stat
			}
		}
	}

	// Boil to set
	final := make(Stats, len(set))
	i := 0
	for _, val := range set {
		final[i] = val
		i++
	}

	// Grab top 3 reviewers and return string lines
	var buffer bytes.Buffer
	maxStats := 3
	if l := len(final); l < maxStats {
		maxStats = l
	}

	topN := chooseTopN(maxStats, final)

	for i := range topN {
		buffer.WriteString(topN[i].String())
		buffer.WriteString("\n")
	}

	return buffer.String(), nil
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

func mergeChans(cs ...chan *Stat) chan *Stat {
	// https://blog.golang.org/pipelines
	var wg sync.WaitGroup
	out := make(chan *Stat)

	output := func(c <-chan *Stat) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))

	for _, c := range cs {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
