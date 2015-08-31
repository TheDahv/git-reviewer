package gitreviewers

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestBranchBehind(t *testing.T) {
	var (
		currentBranch   string
		requiresUnstash = true
		rg              runGuard
	)

	r := Reviewer{}

	// Get current branch
	rg.maybeRun(func() {
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
	rg.maybeRun(func() {
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
	rg.maybeRun(func() {
		err := exec.Command("git", "checkout", "-b", currentBranch+"__behind").Run()
		if err != nil {
			rg.err = err
			rg.msg = "Unable to checkout a new branch"
		}
	})

	// Drop a few commits
	rg.maybeRun(func() {
		err := exec.Command("git", "reset", "--hard", "master~1").Run()
		if err != nil {
			rg.err = err
			rg.msg = "Problem dropping commits"
		}
	})

	// Check branch state
	rg.maybeRun(func() {
		behind, err := r.BranchBehind()

		if err != nil {
			t.Errorf("Got error '%v', expected none\n", err)
			t.FailNow()
		}

		if !behind {
			t.Error("Got false, expectd true")
		}
	})

	// Go back to original working branch
	rg.maybeRun(func() {
		err := exec.Command("git", "checkout", currentBranch).Run()
		if err != nil {
			rg.err = err
			rg.msg = "Unable to switch back to original working branch"
		}
	})

	// Restore workspace
	rg.maybeRun(func() {
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
	rg.maybeRun(func() {
		err := exec.Command("git", "branch", "-D", currentBranch+"__behind").Run()
		if err != nil {
			rg.err = err
			rg.msg = "Unable to destroy test branch"
		}
	})

	if rg.err != nil {
		t.Errorf("Test setup/teardown failed on step %s with error: %v\n", rg.msg, rg.err)
	}
}
