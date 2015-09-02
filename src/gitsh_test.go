package gitreviewers

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestChangedFiles(t *testing.T) {
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
		lines, err := changedFiles([]string{})
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

func TestCommitterCounts(t *testing.T) {
	path := os.Getenv("GOPATH") +
		"/src/github.com/thedahv/git-reviewer/src"

	ch := make(chan Stat)
	done := make(chan statResp)

	var err error
	var counts []Stat

	go func(path string) {
		committerCounts(path, "", ch, done)
	}(path)

	for i := 0; i < 1; {
		select {
		case stat := <-ch:
			counts = append(counts, stat)
		case signal := <-done:
			err = signal.err
			i++
		}
	}

	close(ch)
	close(done)

	if err != nil {
		t.Errorf("Got error %v, expected none\n", err)
		t.FailNow()
	}

	if len(counts) == 0 {
		t.Errorf("Got 0 counts, expected more")
		t.FailNow()
	}

	if counts[0].Reviewer == "" || counts[0].Count == 0 {
		t.Errorf("Got empty stats where we didn't expect to")
	}
}

func TestCommitterCountsOnBadPath(t *testing.T) {
	path := "doesn't-exist"

	ch := make(chan Stat)
	done := make(chan statResp)

	var err error
	var counts []Stat

	go func() {
		committerCounts(path, "", ch, done)
	}()

	for i := 0; i < 1; {
		select {
		case stat := <-ch:
			counts = append(counts, stat)
		case signal := <-done:
			err = signal.err
			i++
		}
	}

	if err == nil {
		t.Error("Got no error back, expected one")
	}

	if len(counts) != 0 {
		t.Errorf("Expected no stats back, got %d\n", len(counts))
	}
}

func TestCommitTimestamp(t *testing.T) {
	ts, err := commitTimeStamp("master")

	if err != nil {
		t.Errorf("Got error %v, expected none\n", err)
		t.FailNow()
	}

	tsi, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		t.Errorf("Unable to turn timestamp into integer: %v\n", err)
	}
	// As long as we parse into some kind of date without issue, we're ok
	time.Unix(tsi, 0)
}
