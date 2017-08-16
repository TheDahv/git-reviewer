package gitreviewers

import (
	"bufio"
	"bytes"
	"container/heap"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/pkg/errors"
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
	Mailmap           mailmap
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

// BuildMailmap builds a map of author name/email combinations to determine the
// canonical author for a given line or commit. This is useful if an author
// worked on a project under multiple identiies but we still want to attribute
// all contributions to the same person.
//
// It attempts to open and read from any of the paths specified. If none are
// specified, it will attempt to open ~/.mailmap and read from there.
//
// It will skip over any files it is unable to open without error. If none are
// parsed, it will result in an empty mailmap.
func (r *ContributionCounter) BuildMailmap(paths ...string) {
	// If no paths specified, attempt by guessing that it will be in the user's
	// home path.
	if len(paths) == 0 {
		if path, err := guessUserMailmap(); err == nil {
			paths = append(paths, path)
		}
	}

	if mm, err := readMailmap(paths); err == nil {
		r.Mailmap = mm
	}
}

// Attempt to guess the user's mailmap path by looking for it in the home
// directory.
func guessUserMailmap() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	path := u.HomeDir + "/.mailmap"
	if f, err := os.Open(path); err == nil {
		f.Close()
		return path, nil
	} else {
		return "", err
	}
}

// BranchBehind determines if the current branch is "behind"
// by comparing the current branch HEAD reference to that of the local ref of
// the master branch.
func (r *ContributionCounter) BranchBehind() (bool, error) {
	var (
		behind bool
		h      *plumbing.Reference
		hObj   *object.Commit
		m      *plumbing.Reference
		mObj   *object.Commit
		rg     runGuard
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
		changes object.Changes
		h       *plumbing.Reference
		hc      *object.Commit
		ht      *object.Tree
		m       *plumbing.Reference
		mc      *object.Commit
		mt      *object.Tree
		paths   []string
		rg      runGuard
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
				// Only keep the names that existed in "master" before the change.
				// Otherwise we'll try to 'blame' files that don't exist in master if a
				// file was created or renamed in the development branch.
				n := ch.From.Name
				if len(n) > 0 && considerExt(n, r) && considerPath(n, r) {
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
//
// NOTE: This previously use go-git to create a blame object for each file in
// 'paths', but the performance and concurrency errors proved to make this
// unsuitable for this method. We're falling back to making and parsing shell
// commands to Git to calculate blame statistics.
//
// Relevant src-d/go-git issues
// - https://github.com/src-d/go-git/issues/457
// - https://github.com/src-d/go-git/issues/458
func (r *ContributionCounter) FindReviewers(paths []string) (string, error) {
	var final Stats

	if len(r.Since) == 0 {
		// Calculate 6 months ago from today's date and set the 'since' argument
		r.Since = time.Now().AddDate(0, -6, 0).Format("2006-01-02")
	}

	// Example shell call:
	// git blame -ce 9901bf79f808a8339b9820c08e209f5ec9649bda src/reviewers.go
	linesByCommitter, totalLines, err := r.generateCounts(paths)
	if err != nil {
		return "", err
	}

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

	if len(topN) == 0 {
		return "", noReviewersErr{}
	}

	fmt.Fprintln(tw, "Reviewer\tExperience")
	fmt.Fprintln(tw, "--------\t----------")

	for i := range topN {
		fmt.Fprintf(tw, "%s\t%.2f%%\n", topN[i].Reviewer, topN[i].Percentage*100.0)
	}
	tw.Flush()

	return buffer.String(), nil
}

func (r *ContributionCounter) generateCounts(paths []string) (map[string]float64, uint16, error) {
	var (
		linesByCommitter = make(map[string]float64)
		m                *plumbing.Reference
		mc               *object.Commit
		rg               runGuard
		totalLines       uint16
		wg               sync.WaitGroup
	)

	// Set up tracking for each of these files to be blamed concurrently with
	// results from each reported on a single channel.
	wg.Add(len(paths))
	reporter := make(chan []string)

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
				go func(p string) {
					// A separate run has already indicated a blame error. Skip
					if rg.err != nil {
						rg.msg = "Issue running git blame for " + p
						return
					}

					err := r.runAndReport(p, m.Hash().String(), reporter)
					// Report any errors to the rungroup so future goroutines don't
					// attempt any further processsing.
					if err != nil {
						rg.err = err
					}
				}(p)
			}
		},
	)

	// Bail early from further processing if we couldn't run a git-blame for each
	// path identified
	if rg.err != nil {
		if rg.msg != "" && r.Verbose {
			fmt.Println("Error blaming changed files:", rg.msg)
		}

		return nil, 0, rg.err
	}

	// Collect all the git-blame line responses as they come in. This loop will
	// continue as long as the reporter channel is open. We'll close the channel
	// when all blame processes report they have finished.
	go func() {
		for attributions := range reporter {
			for _, author := range attributions {
				linesByCommitter[author]++
				totalLines++
			}
			wg.Done()
		}
	}()

	wg.Wait()
	close(reporter)

	return linesByCommitter, totalLines, nil
}

// runAndReport executes an external call to git to calculate blame statistics
// for a file at a specific commit (usually "master" or whatever the base branch
// is) and send extracted statistics to the 'reporter' channel.
func (r *ContributionCounter) runAndReport(path string, rev string, reporter chan []string) error {
	out, err := exec.Command("git", "blame", "-ce", rev, path).Output()
	if err != nil {
		return errors.Wrap(err, "unable to execute external git blame command")
	}

	scn := bufio.NewScanner(bytes.NewReader(out))
	var attributions []string

	for scn.Scan() {
		if bi, err := parseBlameLine(scn.Bytes()); err == nil {
			// r.Since is a string, not a date. However, since the format is just
			// a "YYYY-MM-DD" string, we can rely on ASCII sorting and just compare
			// the strings to determine if a line change was committed before or after
			// our boundary
			if r.Since > string(bi.date) {
				continue
			}

			email := string(bi.email)
			// Normalize scanned email based on what we found in the mailmap
			if e, ok := r.Mailmap[email]; ok {
				attributions = append(attributions, e)
			} else {
				attributions = append(attributions, email)
			}
		} else {
			return errors.Wrap(err, "issue parsing a line in git blame output")
		}
	}

	reporter <- attributions
	return scn.Err()
}

// blameInfo holds anything we might be interested in reporting out of a git
// blame shell command result
type blameInfo struct {
	email []byte
	date  []byte
}

// parseBlameLine takes the bytes for one line of the output of running git
// blame on the shell with the `-ce` options (that is, returning in a specific
// machine format as well as returning the author email instead of name) and
// extracts the relevant information into a blameInfo struct
func parseBlameLine(line []byte) (blameInfo, error) {
	// Format of blame result:
	// somerev        (author@domain.com> YYYY-MM-DD HH:MM:SS -0700       3)stuff.
	var (
		bi    blameInfo
		date  []byte
		email []byte
	)
	rdr := bytes.NewReader(line)

	// Scan over the rev
	for {
		if r, _, err := rdr.ReadRune(); err == nil {
			if r == ' ' || r == '\t' {
				rdr.UnreadRune()
				break
			}
		} else {
			return bi, errors.Wrap(err, "unable to read over rev")
		}
	}

	// Scan over the whitespace gap
	for {
		r, _, err := rdr.ReadRune()
		if err != nil {
			return bi, errors.Wrap(err, "unable to skip whitespace before author")
		}

		if !(r == ' ' || r == '\t') {
			rdr.UnreadRune()
			break
		}
	}

	// Read over author signature header
	if r, _, _ := rdr.ReadRune(); r != '(' {
		return bi, fmt.Errorf("expected opening parens of email")
	}
	if r, _, _ := rdr.ReadRune(); r != '<' {
		return bi, fmt.Errorf("expected opening bracket of email")
	}

	// Scan the email bytes into place
	for {
		b, err := rdr.ReadByte()
		if err != nil {
			return bi, errors.Wrap(err, "unable to scan author email")
		}

		if b == '>' {
			rdr.UnreadRune()
			break
		}

		email = append(email, b)
	}

	// Read over the next space before reading the date
	if r, _, err := rdr.ReadRune(); err != nil || !(r == ' ' || r == '\t') {
		fmt.Println("Error reading expected space after email")
		fmt.Println("Instead of space, got", string(r))
		return bi, err
	}

	// Read the date into place (10 bytes for YYYY-MM-DD)
	for i := 0; i < 10; i++ {
		b, err := rdr.ReadByte()
		if err != nil {
			return bi, errors.Wrap(err, "unable to read date bytes")
		}

		date = append(date, b)
	}

	bi = blameInfo{email, date}
	return bi, nil
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

type NoReviewersErr interface {
	Error() string
	Help() string
}

type noReviewersErr struct{}

func (nre noReviewersErr) Error() string {
	return "no reviewers found"
}

func (nre noReviewersErr) Help() string {
	return "Try using a wider date range"
}
