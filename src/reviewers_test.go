package gitreviewers

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

func TestMergeStats(t *testing.T) {
	statGroups := []Stats{
		Stats{
			Stat{"a", 1},
			Stat{"b", 2},
		},
		Stats{
			Stat{"a", 3},
			Stat{"c", 5},
		},
	}

	expected := Stats{
		Stat{"b", 2},
		Stat{"a", 4},
		Stat{"c", 5},
	}
	actual := mergeStats(statGroups)
	sort.Sort(actual)

	for i, actualStat := range actual {
		if actualStat != expected[i] {
			t.Errorf("Got\n\t%v\n...expected\n\t%v\n", actualStat, expected[i])
		}
	}
}

type runGuard struct {
	err error
	msg string
}

func maybeRun(rg runGuard, fn func()) {
	if rg.err != nil {
		return
	}

	fn()
}

func TestBranchBehind(t *testing.T) {
	var (
		currentBranch   string
		requiresUnstash bool
		rg              runGuard
	)

	requiresUnstash = true

	// Get current branch
	maybeRun(rg, func() {
		out, err := exec.Command("git", "status", "-sb").Output()
		if err != nil {
			rg.err = err
			rg.msg = "Unable to determine current branch"
		}

		// git status -sb format:
		// ## branch_name\nsome other stuff
		nlPos := 0
		for i, b := range out {
			nlPos = i
			if b == '\n' {
				break
			}
		}
		currentBranch = string(out[3:nlPos])
	})

	// Stash existing workspace
	maybeRun(rg, func() {
		out, err := exec.Command("git", "stash", "-u").Output()
		if err != nil {
			rg.err = err
			rg.msg = "Unable to stash workspace"
		}

		if strings.TrimSpace(string(out)) == "No local changes to save" {
			requiresUnstash = false
		}
	})

	// Checkout to new branch
	maybeRun(rg, func() {
		err := exec.Command("git", "checkout", "-b", currentBranch+"__behind").Run()
		if err != nil {
			rg.err = err
			rg.msg = "Unable to checkout a new branch"
		}
	})

	// Drop a few commits
	maybeRun(rg, func() {
		err := exec.Command("git", "reset", "--hard", "master~1").Run()
		if err != nil {
			rg.err = err
			rg.msg = "Problem dropping commits"
		}
	})

	// Check branch state
	maybeRun(rg, func() {
		behind, err := BranchBehind()

		if err != nil {
			t.Errorf("Got error '%v', expected none\n", err)
			t.FailNow()
		}

		if !behind {
			t.Error("Got false, expectd true")
		}
	})

	// Go back to original working branch
	maybeRun(rg, func() {
		err := exec.Command("git", "checkout", currentBranch).Run()
		if err != nil {
			rg.err = err
			rg.msg = "Unable to switch back to original working branch"
		}
	})

	// Restore workspace
	maybeRun(rg, func() {
		if !requiresUnstash {
			// Bail early if we don't have to do any work
			fmt.Println("Skipping unstash")
			return
		}

		err := exec.Command("git", "stash", "pop").Run()
		if err != nil {
			rg.err = err
			rg.msg = "Unable to pop unstaged changes from git stash"
		}
	})

	// Destroy test branch
	maybeRun(rg, func() {
		err := exec.Command("git", "branch", "-D", currentBranch+"__behind").Run()
		if err != nil {
			rg.err = err
			rg.msg = "Unable to destroy test branch"
		}
	})

	if rg.err != nil {
		t.Errorf("Test failed on step %s with error: %v\n", rg.msg, rg.err)
	}
}
