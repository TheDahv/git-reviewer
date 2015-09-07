package gitreviewers

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	gg "github.com/libgit2/git2go"
)

func prepareRepo(t *testing.T) *gg.Repository {
	dir, err := os.Getwd()
	if err != nil {
		t.Errorf("Unable to open repo directory: '%v'\n", err)
		t.FailNow()
	}

	repo, err := gg.OpenRepository(dir + "/..")
	if err != nil {
		t.Errorf("Unable to open repository: '%v'\n", err)
		t.FailNow()
	}

	return repo
}

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

func TestFindFiles(t *testing.T) {
	// Set up a fake commit in a fake branch
	tfName := "fake.co"
	var (
		safeToReset = false
		rg          runGuard
		branch      string
	)

	// Get current branch
	rg.maybeRun(func() {
		out, err := exec.Command("git", "status", "-sb").Output()
		if err != nil {
			rg.err = err
			rg.msg = "Issue getting current branch"
		}
		// Find the newline
		nlPos := 0
		for i, b := range out {
			nlPos = i
			if b == '\n' {
				break
			}
		}
		// git status -sb format:
		// ## branch_name\nsome other stuff
		branch = string(out[3:nlPos])
	})

	// Create test branch
	rg.maybeRun(func() {
		err := exec.Command("git", "checkout", "-b", "fake-branch").Run()

		if err != nil {
			rg.err = err
			rg.msg = "Issue creating new branch. Please clean up!"
		}
	})

	var f *os.File

	// Create a test file
	rg.maybeRun(func() {
		file, err := os.Create(tfName)
		if err != nil {
			rg.err = err
			rg.msg = "Issue setting up fake commit file. Please clean up!"
		} else {
			f = file
		}
	})
	defer func() {
		if f != nil {
			_ = os.Remove(f.Name())
		}
	}()

	// Stage the fake file
	rg.maybeRun(func() {
		err := exec.Command("git", "add", tfName).Run()
		if err != nil {
			rg.err = err
			rg.msg = "Issue staging the commit. Please clean up!"
		}
	})

	// Commit the fake file
	rg.maybeRun(func() {
		err := exec.Command("git", "commit", "-m", "\"Fake commit\"").Run()
		if err != nil {
			rg.err = err
			rg.msg = "Issue committing. Please clean up!"
		}
		safeToReset = true
	})

	// Test for changes
	rg.maybeRun(func() {
		repo := prepareRepo(t)
		defer repo.Free()

		r := Reviewer{Repo: repo}

		lines, err := r.FindFiles()
		if err != nil {
			t.Errorf("Got error %v, expected none\n", err)
		}

		if len(lines) == 0 {
			t.Error("Got 0 lines, expected more")
		}
	})

	// Clean up

	// Switch back to original branch
	rg.maybeRun(func() {
		if safeToReset {
			if err := exec.Command("git", "checkout", branch).Run(); err != nil {
				rg.err = err
				rg.msg = fmt.Sprintf("Issue switching back to %s. Please clean up!", branch)
			}
		}
	})

	// Destroy test branch
	rg.maybeRun(func() {
		if err := exec.Command("git", "branch", "-D", "fake-branch").Run(); err != nil {
			rg.err = err
			rg.msg = "Issue destroying test branch. Please clean up!"
		}
	})

	if rg.err != nil {
		t.Errorf("Test setup/teardown failed on step %s with error: %v\n", rg.msg, rg.err)
	}
}

func TestDefaultIgnoreExtensions(t *testing.T) {
	// All defaults
	if considerExt("myfile.svg", &Reviewer{}) {
		t.Error("Expected SVG files to be ignored by default")
	}

	if considerExt("myfile.json", &Reviewer{}) {
		t.Error("Expected JSON files to be ignored by default")
	}

	if considerExt("myfile.nock", &Reviewer{}) {
		t.Error("Expected NOCK files to be ignored by default")
	}

	if considerExt("myfile.xml", &Reviewer{}) {
		t.Error("Expected XML files to be ignored by default")
	}

	// Defaults in addition to extra extensions
	opts := &Reviewer{IgnoredExtensions: []string{"coffee"}}
	if considerExt("myfile.coffee", opts) {
		t.Error("Expected coffee files to be explicitly ignored")
	}

	if considerExt("myfile.json", opts) {
		t.Error("Expected JSON files to be ignored when other ignores defined")
	}
}
