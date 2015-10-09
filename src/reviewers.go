package gitreviewers

import (
	"bufio"
	"bytes"
	"container/heap"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	gg "github.com/thedahv/git2go"
)

var (
	mailmap    map[string]string
	useMailmap = false
)

func init() {
	var (
		rg  runGuard
		cwd string
	)

	rg.maybeRun(func() {
		var err error

		cwd, err = os.Getwd()
		if err != nil {
			rg.err = err
			rg.msg = "Error determining current working directory"
		}
	})

	mailmap = make(map[string]string)

	// Check for profile mailmap
	rg.maybeRun(func() {
		cp, err := gg.ConfigFindGlobal()
		if err != nil {
			fmt.Printf("Error finding global config: %v\n", err)
			return
		}

		conf, err := gg.OpenOndisk(&gg.Config{}, cp)
		if err != nil {
			return
		}
		defer conf.Free()

		path, err := conf.LookupString("mailmap.file")
		if err != nil {
			fmt.Printf("Error lookup up mailmap file %v\n", err)
			return
		}

		f, err := os.Open(path)
		defer f.Close()
		if err != nil {
			return
		}

		readMailmap(mailmap, f)
		useMailmap = true
	})

	// Parse project mailmap last so it overrides
	rg.maybeRun(func() {
		f, err := os.Open(cwd + ".mailmap")
		defer f.Close()
		if err != nil {
			return
		}

		readMailmap(mailmap, f)
		useMailmap = true
	})

	// Check for any errors that might have happened
	if rg.err != nil {
		useMailmap = false
		fmt.Println("Error starting up! We can't read the mailmap file!")
		fmt.Println(rg.err.Error())

		// If we're left with an error by now, we should stop
		panic(rg.err)
	}
}

func readMailmap(mm map[string]string, f *os.File) {
	// See git C implementation of parse_name_and_email for reference
	// https://github.com/git/git/blob/master/mailmap.c
	var (
		line    []byte
		err     error
		lastPos int
	)
	var name1, email1, name2, email2 string

	rdr := bufio.NewReader(f)

	for {
		line, err = rdr.ReadBytes('\n')
		if err != nil {
			// TODO Handle non-EOF errors
			break
		}

		if line[0] != '#' {
			name1, email1, lastPos = parseMailmapLine(line, 0)

			if lastPos > 0 {
				name2, email2, _ = parseMailmapLine(line, lastPos)

				mm[name2] = name1
				mm[email2] = email1
			}
		}
		// TODO Implement repo-abbrev parsing. I have no idea what that is
	}
}

func parseMailmapLine(line []byte, offset int) (name string, email string, right int) {
	var left int

	left = bytes.IndexRune(line[offset:], '<')
	if left < 0 {
		return
	}

	right = bytes.IndexRune(line[offset+left:], '>')
	if right < 0 {
		return
	}
	// Account for the fact we got the index of a sub-slice
	right = left + right

	name = string(bytes.TrimSpace(line[:offset+left]))
	email = string(bytes.TrimSpace(line[offset+left+1 : offset+right]))

	return
}

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
		rg        runGuard
		rw        *gg.RevWalk
		since     time.Time
		reviewers map[string]int
		final     Stats
	)

	reviewers = make(map[string]int)

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

	// For each of our commits in the review period, see if it affects
	// at least one of the paths changed in the branch. If so, the commit
	// author is added to the count of contributors with experience with one
	// of the changed files in our branch.
	rg.maybeRun(func() {
		var err error

		// Revwalk.Iterate walks through commits until the
		// RevWalkIterator returns false.
		err = rw.Iterate(func(c *gg.Commit) bool {
			var (
				err  error
				tree *gg.Tree
			)
			defer c.Free()

			sig := c.Committer()

			// Stop walking commits since we've passed 'since'
			if sig.When.Before(since) {
				return false
			}

			tree, err = c.Tree()
			if err != nil {
				rg.err = err
				return false
			}

			// Check desired paths to see if one exists in the commit tree
			for _, p := range paths {
				te, err := tree.EntryByPath(p)
				if err != nil {
					continue
				}

				if te != nil {
					k := reviewerKey(sig)
					if _, ok := reviewers[k]; ok {
						reviewers[k]++
					} else {
						reviewers[k] = 1
					}

					// We found a path on the commit, no need to double-count
					break
				}
			}

			return true
		})

		if err != nil {
			rg.err = err
			rg.msg = "Error iterating through rev walk"
		}
	})

	if rg.err != nil {
		fmt.Println(rg.msg)
		fmt.Println(rg.err)

		return "", rg.err
	}

	final = make(Stats, len(reviewers))
	idx := 0
	for reviewer, count := range reviewers {
		final[idx] = &Stat{reviewer, count}
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

func reviewerKey(sig *gg.Signature) string {
	var name, email string

	if useMailmap {
		var ok bool
		if name, ok = mailmap[sig.Name]; ok == false {
			name = sig.Name
		}
		if email, ok = mailmap[sig.Email]; ok == false {
			name = sig.Email
		}
	} else {
		name = sig.Name
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
